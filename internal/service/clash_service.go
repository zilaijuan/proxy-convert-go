package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"proxy-convert/internal/database"
	"proxy-convert/internal/logger"
	"proxy-convert/internal/parser"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type ClashConfig struct {
	Mode               string                   `json:"mode"`
	IPv6               bool                     `json:"ipv6"`
	AllowLan           bool                     `json:"allow-lan"`
	BindAddress        string                   `json:"bind-address"`
	MixedPort          int                      `json:"mixed-port"`
	LogLevel           string                   `json:"log-level"`
	UnifiedDelay       bool                     `json:"unified-delay"`
	TCPConcurrent      bool                     `json:"tcp-concurrent"`
	ExternalController string                   `json:"external-controller"`
	Tun                map[string]interface{}   `json:"tun"`
	DNS                map[string]interface{}   `json:"dns"`
	Proxies            []map[string]interface{} `json:"proxies"`
	ProxyGroups        []map[string]interface{} `json:"proxy-groups"`
	Rules              []string                 `json:"rules"`
	RuleProviders      map[string]interface{}   `json:"rule-providers"`
	URLRewrite         []string                 `json:"url-rewrite"`
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
			logger.Printf("解析link %d 失败: %v, Link内容: %s", link.ID, err, link.Link)
			continue
		}

		proxyMap := buildProxyMap(proxy)
		if proxyMap != nil {
			// 检查 name 是否包含 _CN_
			if name, ok := proxyMap["name"].(string); ok && strings.Contains(name, "_CN_") {
				continue
			}
			// 为 Clash 添加 udp 支持
			if proxy.Type == "ss" {
				proxyMap["udp"] = true
			}
			proxies = append(proxies, proxyMap)
		}
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
			logger.Printf("名称冲突，重命名: %s -> %s", name, newName)
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
	logger.Printf("加载Clash模板: %s", templatePath)

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

	logger.Printf("Clash配置已导出到: %s", outputPath)
	return nil
}

func (s *ClashService) ExportClashConfigYAML(statuses []int) ([]byte, error) {
	config, err := s.BuildClash(statuses)
	if err != nil {
		return nil, err
	}

	// 使用 yaml.Marshal 生成 YAML 数据
	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("序列化YAML失败: %w", err)
	}

	// 将 Unicode 转义序列替换为实际的 emoji 字符
	yamlStr := string(data)

	// 正则表达式匹配 Unicode 转义序列 \UXXXXXXXX
	re := regexp.MustCompile(`\\U([0-9A-Fa-f]{8})`)

	// 替换转义序列为实际的 Unicode 字符
	result := re.ReplaceAllStringFunc(yamlStr, func(m string) string {
		// 移除 \U 前缀，获取十六进制值
		hex := m[2:]

		// 将十六进制值转换为整数
		codePoint, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return m // 转换失败，保持原样
		}

		// 将整数转换为 Unicode 字符并返回
		return string(rune(codePoint))
	})

	return []byte(result), nil
}
