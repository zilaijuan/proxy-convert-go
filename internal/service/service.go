package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"proxy-convert/internal/database"
	"proxy-convert/internal/extractor"
	"proxy-convert/internal/parser"
	"time"

	"gopkg.in/yaml.v3"
)

type LinkService struct {
	db *database.DB
}

func NewLinkService(db *database.DB) *LinkService {
	return &LinkService{db: db}
}

func (s *LinkService) AddLink(link string, status int) (int64, error) {
	proxy, err := parser.ParseLink(link)
	if err != nil {
		log.Printf("解析link失败: %v", err)
		return s.db.AddLink(link, status, "", "")
	}

	fingerprint := parser.GetNodeFingerprint(proxy)
	name := proxy.Name

	return s.db.AddLink(link, status, fingerprint, name)
}

func (s *LinkService) GetAllLinks(statuses []int, limit, offset int) ([]database.Link, error) {
	if limit == 0 {
		limit = 1000
	}
	return s.db.GetAllLinks(statuses, limit, offset)
}

func (s *LinkService) GetLink(id int) (*database.Link, error) {
	return s.db.GetLink(id)
}

func (s *LinkService) UpdateLinkStatus(id int, status int) (bool, error) {
	return s.db.UpdateLink(id, nil, &status, nil)
}

func (s *LinkService) DeleteLink(id int) (bool, error) {
	return s.db.DeleteLink(id)
}

func (s *LinkService) CountLinks(statuses []int) (int, error) {
	return s.db.CountLinks(statuses)
}

type VerifierService struct {
	db                  *database.DB
	mihomoPath          string
	testURL             string
	timeout             time.Duration
	externalControllerPort int
}

type NodeDelayTest struct {
	Delay int `json:"delay"`
}

func NewVerifierService(db *database.DB) *VerifierService {
	mihomoPath := "./mihomo.exe"
	if path := os.Getenv("MIHOMO_PATH"); path != "" {
		mihomoPath = path
	}

	testURL := "http://www.google.com"
	if url := os.Getenv("TEST_URL"); url != "" {
		testURL = url
	}

	timeout := 10 * time.Second
	if timeoutStr := os.Getenv("TIMEOUT"); timeoutStr != "" {
		if t, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = t
		}
	}

	return &VerifierService{
		db:                  db,
		mihomoPath:          mihomoPath,
		testURL:             testURL,
		timeout:             timeout,
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

func (s *VerifierService) buildProxyMap(proxy *parser.Proxy) map[string]interface{} {
	proxyMap := map[string]interface{}{
		"name":   proxy.Name,
		"type":   proxy.Type,
		"server": proxy.Server,
		"port":   proxy.Port,
	}

	switch proxy.Type {
	case "ss":
		if cipher, ok := proxy.Extra["cipher"].(string); ok {
			proxyMap["cipher"] = cipher
		}
		if password, ok := proxy.Extra["password"].(string); ok {
			proxyMap["password"] = password
		}
	case "vmess":
		if uuid, ok := proxy.Extra["uuid"].(string); ok {
			proxyMap["uuid"] = uuid
		}
		if alterId, ok := proxy.Extra["alterId"].(int); ok {
			proxyMap["alterId"] = alterId
		}
		if cipher, ok := proxy.Extra["cipher"].(string); ok {
			proxyMap["cipher"] = cipher
		}
		if network, ok := proxy.Extra["network"].(string); ok {
			proxyMap["network"] = network
		}
		if tls, ok := proxy.Extra["tls"].(bool); ok && tls {
			proxyMap["tls"] = true
			if servername, ok := proxy.Extra["servername"].(string); ok {
				proxyMap["servername"] = servername
			}
		}
		if wsOpts, ok := proxy.Extra["ws-opts"].(map[string]interface{}); ok {
			proxyMap["ws-opts"] = wsOpts
		}
	case "vless":
		if uuid, ok := proxy.Extra["uuid"].(string); ok {
			proxyMap["uuid"] = uuid
		}
		if network, ok := proxy.Extra["network"].(string); ok {
			proxyMap["network"] = network
		}
		if tls, ok := proxy.Extra["tls"].(bool); ok && tls {
			proxyMap["tls"] = true
			if servername, ok := proxy.Extra["servername"].(string); ok {
				proxyMap["servername"] = servername
			}
		}
		if flow, ok := proxy.Extra["flow"].(string); ok {
			proxyMap["flow"] = flow
		}
		if wsOpts, ok := proxy.Extra["ws-opts"].(map[string]interface{}); ok {
			proxyMap["ws-opts"] = wsOpts
		}
	case "trojan":
		if password, ok := proxy.Extra["password"].(string); ok {
			proxyMap["password"] = password
		}
		if sni, ok := proxy.Extra["sni"].(string); ok {
			proxyMap["sni"] = sni
		}
	case "hysteria2", "hy2":
		if authStr, ok := proxy.Extra["auth_str"].(string); ok {
			proxyMap["password"] = authStr
		}
		if sni, ok := proxy.Extra["sni"].(string); ok {
			proxyMap["sni"] = sni
		}
	default:
		return nil
	}

	return proxyMap
}

func (s *VerifierService) writeConfig(configPath string, proxies []map[string]interface{}, proxyNames []string, externalControllerPort int) error {
	config := map[string]interface{}{
		"mixed-port":           7890,
		"allow-lan":            false,
		"mode":                 "rule",
		"log-level":            "info",
		"external-controller": fmt.Sprintf("127.0.0.1:%d", externalControllerPort),
		"proxies":              proxies,
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

	log.Printf("Writing config to %s", configPath)
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
	links, err := s.db.GetAllLinks([]int{0, 1}, 0, 0)
	if err != nil {
		return fmt.Errorf("获取links失败: %w", err)
	}

	if len(links) == 0 {
		log.Println("No links to verify")
		return nil
	}

	log.Printf("Found %d links to verify", len(links))

	tempDir, err := os.MkdirTemp("", "mihomo-verifier-")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")

	proxies := make([]map[string]interface{}, 0, len(links))
	proxyNames := make([]string, 0, len(links))
	nameCount := make(map[string]int)
	nameMap := make(map[string]string)
	linkMap := make(map[string]database.Link)

	for _, link := range links {
		proxy, err := parser.ParseLink(link.Link)
		if err != nil {
			log.Printf("解析link %d 失败: %v", link.ID, err)
			continue
		}

		proxyMap := s.buildProxyMap(proxy)
		if proxyMap != nil {
			originalName := proxy.Name
			safeBase := s.sanitizeName(originalName)
			count := nameCount[safeBase]
			nameCount[safeBase]++
			
			var safeName string
			if count > 0 {
				safeName = fmt.Sprintf("%s_%d", safeBase, count)
			} else {
				safeName = safeBase
			}
			
			proxyMap["name"] = safeName
			proxies = append(proxies, proxyMap)
			proxyNames = append(proxyNames, safeName)
			nameMap[safeName] = originalName
			linkMap[safeName] = link
			
			log.Printf("Proxy: original=%q, safe=%q", originalName, safeName)
		}
	}

	if len(proxies) == 0 {
		log.Println("No valid proxies to test")
		return nil
	}

	err = s.writeConfig(configPath, proxies, proxyNames, s.externalControllerPort)
	if err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	log.Println("Starting mihomo...")
	cmd := exec.Command(s.mihomoPath, "-d", tempDir)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("启动mihomo失败: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			log.Println("Killing mihomo process...")
			cmd.Process.Kill()
		}
	}()

	log.Println("Waiting for mihomo to start...")
	ready := false
	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		
		client := &http.Client{
			Timeout: 2 * time.Second,
		}
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d", s.externalControllerPort))
		if err == nil {
			resp.Body.Close()
			ready = true
			log.Println("mihomo is ready!")
			break
		}
		log.Printf("Waiting for mihomo... (%d/30)", i+1)
	}

	if !ready {
		log.Printf("mihomo stdout: %s", stdout.String())
		log.Printf("mihomo stderr: %s", stderr.String())
		return fmt.Errorf("mihomo在30秒内启动失败")
	}

	log.Println("Testing node delays...")
	results := make(map[string]int)

	for _, proxyName := range proxyNames {
		delay, err := s.testNodeDelay(s.externalControllerPort, proxyName, s.testURL, s.timeout)
		if err != nil {
			log.Printf("Failed to test node %s: %v", proxyName, err)
			results[proxyName] = -1
		} else {
			log.Printf("Node %s delay: %dms", proxyName, delay)
			results[proxyName] = delay
		}
	}

	log.Println("Updating link statuses...")
	successCount := 0
	failCount := 0

	reverseNameMap := make(map[string]string)
	for safeName, originalName := range nameMap {
		reverseNameMap[originalName] = safeName
	}

	for _, link := range links {
		proxy, err := parser.ParseLink(link.Link)
		if err != nil {
			continue
		}

		safeName, ok := reverseNameMap[proxy.Name]
		if !ok {
			continue
		}

		delay, ok := results[safeName]
		if !ok {
			continue
		}

		newStatus := -1
		if delay > 0 && delay < int(s.timeout.Milliseconds()) {
			newStatus = 1
			successCount++
		} else {
			failCount++
		}

		_, err = s.db.UpdateLink(link.ID, nil, &newStatus, nil)
		if err != nil {
			log.Printf("更新link %d 失败: %v", link.ID, err)
		}
	}

	log.Printf("Verification completed: %d successful, %d failed", successCount, failCount)
	return nil
}

type ExtractorService struct {
	db *database.DB
}

func NewExtractorService(db *database.DB) *ExtractorService {
	return &ExtractorService{db: db}
}

func (s *ExtractorService) ExtractFromV2rayse() error {
	extractor := extractor.NewV2rayseExtractor()
	links := extractor.Run(nil)

	importedCount := 0
	existingCount := 0

	for _, link := range links {
		id, err := s.db.AddLink(link, 0, "", "")
		if err != nil {
			existingCount++
		} else {
			importedCount++
			log.Printf("导入link ID: %d", id)
		}
	}

	log.Printf("链接导入完成: 新增 %d 个, 已存在 %d 个", importedCount, existingCount)
	return nil
}

func (s *ExtractorService) ExtractFromGitHub() error {
	extractor := extractor.NewGitHubExtractor()
	links := extractor.Run(nil)

	importedCount := 0
	existingCount := 0

	for _, link := range links {
		id, err := s.db.AddLink(link, 0, "", "")
		if err != nil {
			existingCount++
		} else {
			importedCount++
			log.Printf("导入link ID: %d", id)
		}
	}

	log.Printf("GitHub链接导入完成: 新增 %d 个, 已存在 %d 个", importedCount, existingCount)
	return nil
}

func (s *ExtractorService) ImportFromURL(url string) error {
	contentParser := extractor.NewContentParser()

	client := &http.Client{}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("获取URL内容失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应内容失败: %w", err)
	}

	links := contentParser.ParseContent(string(body))

	importedCount := 0
	existingCount := 0

	for _, link := range links {
		_, err := s.db.AddLink(link, 0, "", "")
		if err != nil {
			existingCount++
		} else {
			importedCount++
		}
	}

	log.Printf("链接导入完成: 新增 %d 个, 已存在 %d 个", importedCount, existingCount)
	return nil
}

func (s *ExtractorService) ImportFromText(text string) error {
	contentParser := extractor.NewContentParser()
	links := contentParser.ParseContent(text)

	importedCount := 0
	existingCount := 0

	for _, link := range links {
		_, err := s.db.AddLink(link, 0, "", "")
		if err != nil {
			existingCount++
		} else {
			importedCount++
		}
	}

	log.Printf("链接导入完成: 新增 %d 个, 已存在 %d 个", importedCount, existingCount)
	return nil
}

type ClashConfig struct {
	Mode               string                 `json:"mode"`
	IPv6               bool                   `json:"ipv6"`
	AllowLan           bool                   `json:"allow-lan"`
	BindAddress        string                 `json:"bind-address"`
	MixedPort          int                    `json:"mixed-port"`
	LogLevel           string                 `json:"log-level"`
	UnifiedDelay       bool                   `json:"unified-delay"`
	TCPConcurrent      bool                   `json:"tcp-concurrent"`
	ExternalController string                 `json:"external-controller"`
	Tun                map[string]interface{} `json:"tun"`
	DNS                map[string]interface{} `json:"dns"`
	Proxies            []map[string]interface{} `json:"proxies"`
	ProxyGroups        []map[string]interface{} `json:"proxy-groups"`
	Rules              []string               `json:"rules"`
	RuleProviders      map[string]interface{} `json:"rule-providers"`
	URLRewrite         []string               `json:"url-rewrite"`
}

type ClashService struct {
	db *database.DB
}

func NewClashService(db *database.DB) *ClashService {
	return &ClashService{db: db}
}

func (s *ClashService) BuildClash(statuses []int) (map[string]interface{}, error) {
	links, err := s.db.GetAllLinks(statuses, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("获取links失败: %w", err)
	}

	proxies := make([]map[string]interface{}, 0, len(links))
	for _, link := range links {
		proxy, err := parser.ParseLink(link.Link)
		if err != nil {
			log.Printf("解析link失败: %v", err)
			continue
		}

		proxyMap := map[string]interface{}{
			"name":   proxy.Name,
			"type":   proxy.Type,
			"server": proxy.Server,
			"port":   proxy.Port,
		}

		switch proxy.Type {
		case "ss":
			if cipher, ok := proxy.Extra["cipher"].(string); ok {
				proxyMap["cipher"] = cipher
			}
			if password, ok := proxy.Extra["password"].(string); ok {
				proxyMap["password"] = password
			}
			proxyMap["udp"] = true
		case "vmess":
			if uuid, ok := proxy.Extra["uuid"].(string); ok {
				proxyMap["uuid"] = uuid
			}
			if alterId, ok := proxy.Extra["alterId"].(int); ok {
				proxyMap["alterId"] = alterId
			}
			if cipher, ok := proxy.Extra["cipher"].(string); ok {
				proxyMap["cipher"] = cipher
			}
			if network, ok := proxy.Extra["network"].(string); ok {
				proxyMap["network"] = network
			}
			if tls, ok := proxy.Extra["tls"].(bool); ok && tls {
				proxyMap["tls"] = true
				if servername, ok := proxy.Extra["servername"].(string); ok {
					proxyMap["servername"] = servername
				}
			}
			if wsOpts, ok := proxy.Extra["ws-opts"].(map[string]interface{}); ok {
				proxyMap["ws-opts"] = wsOpts
			}
		case "vless":
			if uuid, ok := proxy.Extra["uuid"].(string); ok {
				proxyMap["uuid"] = uuid
			}
			if network, ok := proxy.Extra["network"].(string); ok {
				proxyMap["network"] = network
			}
			if tls, ok := proxy.Extra["tls"].(bool); ok && tls {
				proxyMap["tls"] = true
				if servername, ok := proxy.Extra["servername"].(string); ok {
					proxyMap["servername"] = servername
				}
			}
			if flow, ok := proxy.Extra["flow"].(string); ok {
				proxyMap["flow"] = flow
			}
			if wsOpts, ok := proxy.Extra["ws-opts"].(map[string]interface{}); ok {
				proxyMap["ws-opts"] = wsOpts
			}
		case "trojan":
			if password, ok := proxy.Extra["password"].(string); ok {
				proxyMap["password"] = password
			}
			if sni, ok := proxy.Extra["sni"].(string); ok {
				proxyMap["sni"] = sni
			}
		case "hysteria2", "hy2":
			if authStr, ok := proxy.Extra["auth_str"].(string); ok {
				proxyMap["password"] = authStr
			}
			if sni, ok := proxy.Extra["sni"].(string); ok {
				proxyMap["sni"] = sni
			}
		}

		proxies = append(proxies, proxyMap)
	}

	proxies = s.fixDuplicateNames(proxies)

	template, err := s.loadClashTemplate()
	if err != nil {
		return nil, fmt.Errorf("加载Clash模板失败: %w", err)
	}

	template["proxies"] = proxies

	proxyNames := make([]string, len(proxies))
	for i, p := range proxies {
		proxyNames[i] = p["name"].(string)
	}

	if proxyGroups, ok := template["proxy-groups"].([]interface{}); ok {
		for _, pg := range proxyGroups {
			if pgMap, ok := pg.(map[string]interface{}); ok {
				if name, ok := pgMap["name"].(string); ok {
					if name == "🚀 Proxy" || name == "🌏 Auto" {
						if existingProxies, ok := pgMap["proxies"].([]interface{}); ok {
							newProxies := make([]interface{}, len(existingProxies)+len(proxyNames))
							copy(newProxies, existingProxies)
							for i, pn := range proxyNames {
								newProxies[len(existingProxies)+i] = pn
							}
							pgMap["proxies"] = newProxies
						}
					}
				}
			}
		}
	}

	return template, nil
}

func (s *ClashService) fixDuplicateNames(proxies []map[string]interface{}) []map[string]interface{} {
	nameCount := make(map[string]int)
	for _, p := range proxies {
		name, ok := p["name"].(string)
		if !ok {
			name = "proxy"
		}
		if count, exists := nameCount[name]; !exists {
			nameCount[name] = 0
		} else {
			nameCount[name] = count + 1
			newName := fmt.Sprintf("%s_%d", name, nameCount[name])
			log.Printf("名称冲突，重命名: %s -> %s", name, newName)
			p["name"] = newName
		}
	}
	return proxies
}

func (s *ClashService) loadClashTemplate() (map[string]interface{}, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("获取工作目录失败: %w", err)
	}
	
	templatePath := filepath.Join(wd, "clash_template.json")
	log.Printf("加载Clash模板: %s", templatePath)
	
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("读取模板文件失败: %w", err)
	}

	var template map[string]interface{}
	if err := json.Unmarshal(data, &template); err != nil {
		return nil, fmt.Errorf("解析模板JSON失败: %w", err)
	}

	return template, nil
}

func (s *ClashService) ExportClashConfig(statuses []int, outputPath string) error {
	config, err := s.BuildClash(statuses)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	log.Printf("Clash配置已导出到: %s", outputPath)
	return nil
}

func (s *ClashService) ExportClashConfigYAML(statuses []int) ([]byte, error) {
	config, err := s.BuildClash(statuses)
	if err != nil {
		return nil, err
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("序列化YAML失败: %w", err)
	}

	yamlStr := string(data)
	yamlStr = escapeUnicode(yamlStr)
	return []byte(yamlStr), nil
}

func escapeUnicode(s string) string {
	result := ""
	for i := 0; i < len(s); {
		if i+10 <= len(s) && s[i:i+2] == "\\U" {
			hexStr := s[i+2 : i+10]
			if codePoint, err := parseHex(hexStr); err == nil {
				result += string(rune(codePoint))
				i += 10
				continue
			}
		}
		if i+6 <= len(s) && s[i:i+2] == "\\u" {
			hexStr := s[i+2 : i+6]
			if codePoint, err := parseHex(hexStr); err == nil {
				result += string(rune(codePoint))
				i += 6
				continue
			}
		}
		result += string(s[i])
		i++
	}
	return result
}

func parseHex(s string) (int64, error) {
	var result int64
	for _, c := range s {
		var v byte
		switch {
		case '0' <= c && c <= '9':
			v = byte(c - '0')
		case 'a' <= c && c <= 'f':
			v = byte(c - 'a' + 10)
		case 'A' <= c && c <= 'F':
			v = byte(c - 'A' + 10)
		default:
			return 0, fmt.Errorf("invalid hex")
		}
		result = result<<4 | int64(v)
	}
	return result, nil
}
