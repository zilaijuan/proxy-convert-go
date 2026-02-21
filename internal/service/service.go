package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"proxy-convert/internal/database"
	"proxy-convert/internal/extractor"
	"proxy-convert/internal/parser"
	"sync"
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

func (s *LinkService) GetAllLinks(status *int, limit, offset int) ([]database.Link, error) {
	if limit == 0 {
		limit = 1000
	}
	return s.db.GetAllLinks(status, limit, offset)
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

func (s *LinkService) CountLinks(status *int) (int, error) {
	return s.db.CountLinks(status)
}

type VerifierService struct {
	db *database.DB
}

func NewVerifierService(db *database.DB) *VerifierService {
	return &VerifierService{db: db}
}

func (s *VerifierService) VerifyLinks() error {
	links, err := s.db.GetAllLinks(nil, 0, 0)
	if err != nil {
		return fmt.Errorf("获取links失败: %w", err)
	}

	log.Printf("开始验证 %d 个节点", len(links))

	linkList := make([]string, len(links))
	for i, link := range links {
		linkList[i] = link.Link
	}

	results := s.verifyNodes(linkList)

	successCount := 0
	failCount := 0

	for _, link := range links {
		isAvailable := results[link.Link]
		newStatus := 0
		if isAvailable {
			newStatus = 1
			successCount++
		} else {
			newStatus = -1
			failCount++
		}

		if _, err := s.db.UpdateLink(link.ID, nil, &newStatus, nil); err != nil {
			log.Printf("更新link状态失败: %v", err)
		}
	}

	log.Printf("Link verification completed: %d total, %d successful, %d failed", len(links), successCount, failCount)
	return nil
}

func (s *VerifierService) verifyNodes(links []string) map[string]bool {
	results := make(map[string]bool)
	var mu sync.Mutex

	var wg sync.WaitGroup
	chunkSize := 10

	for i := 0; i < len(links); i += chunkSize {
		end := i + chunkSize
		if end > len(links) {
			end = len(links)
		}

		chunk := links[i:end]
		wg.Add(1)

		go func(chunk []string) {
			defer wg.Done()

			for _, link := range chunk {
				isAvailable := s.verifyNode(link)
				mu.Lock()
				results[link] = isAvailable
				mu.Unlock()
			}
		}(chunk)
	}

	wg.Wait()
	return results
}

func (s *VerifierService) verifyNode(link string) bool {
	proxy, err := parser.ParseLink(link)
	if err != nil {
		log.Printf("解析节点失败: %v", err)
		return false
	}

	log.Printf("验证节点: %s (%s:%d)", proxy.Name, proxy.Server, proxy.Port)

	supportedProtocols := map[string]bool{
		"vless":     true,
		"mvless":    true,
		"vmess":     true,
		"trojan":    true,
		"ss":        true,
		"socks":     true,
		"wireguard": true,
		"hysteria":  true,
		"hysteria2": true,
		"hy2":       true,
	}

	if !supportedProtocols[proxy.Type] {
		log.Printf("不支持的协议: %s", proxy.Type)
		return true
	}

	return s.testConnection(proxy)
}

func (s *VerifierService) testConnection(proxy *parser.Proxy) bool {
	address := fmt.Sprintf("%s:%d", proxy.Server, proxy.Port)
	timeout := 5 * time.Second

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		log.Printf("连接 %s 失败: %v", address, err)
		return false
	}
	defer conn.Close()

	log.Printf("连接 %s 成功", address)
	return true
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

func (s *ClashService) BuildClash(status *int) (map[string]interface{}, error) {
	links, err := s.db.GetAllLinks(status, 0, 0)
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

func (s *ClashService) ExportClashConfig(status *int, outputPath string) error {
	config, err := s.BuildClash(status)
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

func (s *ClashService) ExportClashConfigYAML(status *int) ([]byte, error) {
	config, err := s.BuildClash(status)
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
