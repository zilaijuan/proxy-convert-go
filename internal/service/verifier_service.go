package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"proxy-convert/internal/config"
	"proxy-convert/internal/database"
	"proxy-convert/internal/logger"
	"proxy-convert/internal/parser"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type VerifierService struct {
	db                     *database.DB
	cfg                    *config.Config
	mihomoPath             string
	testURL                string
	timeout                time.Duration
	externalControllerPort int
}

type NodeDelayTest struct {
	Delay int `json:"delay"`
}

func NewVerifierService(db *database.DB, cfg *config.Config) *VerifierService {
	timeout := 10 * time.Second
	if timeoutStr := os.Getenv("TIMEOUT"); timeoutStr != "" {
		if t, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = t
		}
	}

	return &VerifierService{
		db:                     db,
		cfg:                    cfg,
		mihomoPath:             cfg.Verifier.MihomoPath,
		testURL:                cfg.Verifier.TestURL,
		timeout:                timeout,
		externalControllerPort: 9090,
	}
}

func (s *VerifierService) sanitizeName(name string) string {
	result := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result += string(r)
		} else {
			result += "_"
		}
	}
	if result == "" {
		result = "proxy"
	}
	return result
}

func uniqueProxyName(base string, usedNames map[string]struct{}, nextSuffix map[string]int) string {
	if _, exists := usedNames[base]; !exists {
		usedNames[base] = struct{}{}
		if nextSuffix[base] == 0 {
			nextSuffix[base] = 1
		}
		return base
	}

	suffix := nextSuffix[base]
	if suffix == 0 {
		suffix = 1
	}

	for {
		candidate := fmt.Sprintf("%s_%d", base, suffix)
		if _, exists := usedNames[candidate]; !exists {
			usedNames[candidate] = struct{}{}
			nextSuffix[base] = suffix + 1
			return candidate
		}
		suffix++
	}
}

func (s *VerifierService) writeConfig(configPath string, proxies []map[string]interface{}, proxyNames []string, externalControllerPort int) error {
	config := map[string]interface{}{
		"mixed-port":          7890,
		"allow-lan":           false,
		"mode":                "rule",
		"log-level":           "info",
		"external-controller": fmt.Sprintf("127.0.0.1:%d", externalControllerPort),
		"proxies":             proxies,
		"proxy-groups": []map[string]interface{}{
			{
				"name":    "PROXY",
				"type":    "select",
				"proxies": proxyNames,
			},
		},
		"rules": []string{
			"MATCH,PROXY",
		},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	logger.Printf("Writing config to %s", configPath)
	return os.WriteFile(configPath, data, 0644)
}

func (s *VerifierService) testNodeDelay(port int, proxyName, testURL string, timeout time.Duration) (int, error) {
	client := &http.Client{
		Timeout: timeout,
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/proxies/%s/delay", port, url.PathEscape(proxyName))
	u, err := url.Parse(baseURL)
	if err != nil {
		return -1, err
	}

	q := u.Query()
	q.Set("timeout", fmt.Sprintf("%d", timeout.Milliseconds()))
	q.Set("url", testURL)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return -1, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, err
	}

	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var result NodeDelayTest
	err = json.Unmarshal(body, &result)
	if err != nil {
		return -1, err
	}

	return result.Delay, nil
}

func (s *VerifierService) VerifyLinks() error {
	// 获取最新配置
	latestCfg := config.Get()
	
	// 更新配置
	s.mihomoPath = latestCfg.Verifier.MihomoPath
	s.testURL = latestCfg.Verifier.TestURL
	s.timeout = latestCfg.Verifier.Timeout
	
	links, err := s.db.GetAllLinks([]int{0, 1}, 0, 0)
	if err != nil {
		return fmt.Errorf("get links: %w", err)
	}

	if len(links) == 0 {
		logger.Println("No links to verify")
		return nil
	}

	logger.Printf("Found %d links to verify", len(links))

	tempDir, err := os.MkdirTemp("", "mihomo-verifier-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")

	proxies := make([]map[string]interface{}, 0, len(links))
	proxyNames := make([]string, 0, len(links))
	usedNames := make(map[string]struct{})
	nextSuffix := make(map[string]int)
	linkMap := make(map[string]database.Link)

	for _, link := range links {
		proxy, err := parser.ParseLink(link.Link)
		if err != nil {
			logger.Printf("Failed to parse link %d: %v, content: %s", link.ID, err, link.Link)
			continue
		}

		proxyMap := buildProxyMap(proxy)
		if proxyMap == nil {
			continue
		}

		originalName := proxy.Name
		safeBase := s.sanitizeName(originalName)
		safeName := uniqueProxyName(safeBase, usedNames, nextSuffix)

		proxyMap["name"] = safeName
		proxies = append(proxies, proxyMap)
		proxyNames = append(proxyNames, safeName)
		linkMap[safeName] = link

		logger.Printf("Proxy: original=%q, safe=%q", originalName, safeName)
	}

	if len(proxies) == 0 {
		logger.Println("No valid proxies to test")
		return nil
	}

	err = s.writeConfig(configPath, proxies, proxyNames, s.externalControllerPort)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	logger.Println("Starting mihomo...")
	cmd := exec.Command(s.mihomoPath, "-d", tempDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("start mihomo: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	waitConsumed := false
	consumeWait := func(nonBlocking bool) (bool, error) {
		if waitConsumed {
			return true, nil
		}
		if nonBlocking {
			select {
			case err := <-waitCh:
				waitConsumed = true
				return true, err
			default:
				return false, nil
			}
		}

		err := <-waitCh
		waitConsumed = true
		return true, err
	}

	defer func() {
		if cmd.Process != nil && !waitConsumed {
			logger.Println("Killing mihomo process...")
			_ = cmd.Process.Kill()
			_, _ = consumeWait(false)
		}
	}()

	logger.Println("Waiting for mihomo to start...")
	ready := false
	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	for i := 0; i < 30; i++ {
		if exited, waitErr := consumeWait(true); exited {
			logger.Printf("mihomo stdout: %s", stdout.String())
			logger.Printf("mihomo stderr: %s", stderr.String())
			if waitErr != nil {
				return fmt.Errorf("mihomo exited before it became ready: %w", waitErr)
			}
			return fmt.Errorf("mihomo exited before it became ready")
		}

		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d", s.externalControllerPort))
		if err == nil {
			resp.Body.Close()
			ready = true
			logger.Println("mihomo is ready!")
			break
		}

		logger.Printf("Waiting for mihomo... (%d/30)", i+1)
		time.Sleep(1 * time.Second)
	}

	if !ready {
		if exited, waitErr := consumeWait(true); exited {
			logger.Printf("mihomo stdout: %s", stdout.String())
			logger.Printf("mihomo stderr: %s", stderr.String())
			if waitErr != nil {
				return fmt.Errorf("mihomo exited before it became ready: %w", waitErr)
			}
			return fmt.Errorf("mihomo exited before it became ready")
		}

		logger.Printf("mihomo stdout: %s", stdout.String())
		logger.Printf("mihomo stderr: %s", stderr.String())
		return fmt.Errorf("mihomo did not become ready within 30 seconds")
	}

	logger.Println("Testing node delays...")
	results := make(map[string]int)
	var mu sync.Mutex

	const workerCount = 10
	jobs := make(chan string, len(proxyNames))
	resultsChan := make(chan struct {
		name  string
		delay int
		err   error
	}, len(proxyNames))

	for i := 0; i < workerCount; i++ {
		go func() {
			for proxyName := range jobs {
				delay, err := s.testNodeDelay(s.externalControllerPort, proxyName, s.testURL, s.timeout)
				resultsChan <- struct {
					name  string
					delay int
					err   error
				}{proxyName, delay, err}
			}
		}()
	}

	for _, proxyName := range proxyNames {
		jobs <- proxyName
	}
	close(jobs)

	for i := 0; i < len(proxyNames); i++ {
		result := <-resultsChan
		mu.Lock()
		if result.err != nil {
			logger.Printf("Failed to test node %s: %v", result.name, result.err)
			results[result.name] = -1
		} else {
			logger.Printf("Node %s delay: %dms", result.name, result.delay)
			results[result.name] = result.delay
		}
		mu.Unlock()
	}

	logger.Println("Updating link statuses...")
	successCount := 0
	failCount := 0

	for safeName, link := range linkMap {
		delay, ok := results[safeName]
		if !ok {
			continue
		}

		if delay > 0 && delay < int(s.timeout.Milliseconds()) {
			newStatus := 1
			newCount := 0
			_, err = s.db.UpdateLinkStatusAndCount(link.ID, &newStatus, &newCount)
			if err != nil {
				logger.Printf("Failed to update link %d: %v", link.ID, err)
			} else {
				successCount++
				logger.Printf("Link %d verified successfully, status=1, count=0", link.ID)
			}
			continue
		}

		newCount := link.Count + 1
		var newStatus *int
		if newCount > latestCfg.Verifier.MaxFailCount {
			status := -1
			newStatus = &status
			logger.Printf("Link %d verification failed, count=%d > %d, status=-1", link.ID, newCount, latestCfg.Verifier.MaxFailCount)
		} else {
			status := 0
			newStatus = &status
			logger.Printf("Link %d verification failed, count=%d, status=0", link.ID, newCount)
		}

		_, err = s.db.UpdateLinkStatusAndCount(link.ID, newStatus, &newCount)
		if err != nil {
			logger.Printf("Failed to update link %d: %v", link.ID, err)
		} else {
			failCount++
		}
	}

	logger.Printf("Verification completed: %d successful, %d failed", successCount, failCount)
	return nil
}
