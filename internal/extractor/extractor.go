package extractor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type GitHubExtractor struct {
	timeout time.Duration
}

func NewGitHubExtractor() *GitHubExtractor {
	return &GitHubExtractor{
		timeout: 10 * time.Second,
	}
}

func (e *GitHubExtractor) fetchContent(url string) (string, error) {
	log.Printf("正在访问网址: %s", url)

	client := &http.Client{
		Timeout: e.timeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		log.Printf("访问网址失败: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	log.Println("成功获取内容")
	return string(body), nil
}

func (e *GitHubExtractor) parseContent(content string) []string {
	parser := NewContentParser()
	return parser.ParseContent(content)
}

func (e *GitHubExtractor) Run(urls []string) []string {
	if len(urls) == 0 {
		urls = []string{
			"https://github.com/arshiacomplus/v2rayExtractor/blob/main/mix/sub.html",
		}
	}

	var allLinks []string

	for _, url := range urls {
		content, err := e.fetchContent(url)
		if err != nil {
			log.Printf("获取 %s 失败: %v", url, err)
			continue
		}

		links := e.parseContent(content)
		allLinks = append(allLinks, links...)
	}

	uniqueLinks := e.deduplicate(allLinks)
	log.Printf("总计解码: %d 行, 去重后剩余: %d 行", len(allLinks), len(uniqueLinks))

	return uniqueLinks
}

func (e *GitHubExtractor) deduplicate(links []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, link := range links {
		if !seen[link] {
			seen[link] = true
			result = append(result, link)
		}
	}

	return result
}

type V2rayseExtractor struct {
	timeout        time.Duration
	base64Strings []string
}

func NewV2rayseExtractor() *V2rayseExtractor {
	return &V2rayseExtractor{
		timeout: 10 * time.Second,
	}
}

func (e *V2rayseExtractor) fetchHTML(url string) (string, error) {
	log.Printf("正在访问网址: %s", url)

	client := &http.Client{
		Timeout: e.timeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		log.Printf("访问网址失败: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	log.Println("成功获取HTML内容")
	return string(body), nil
}

func (e *V2rayseExtractor) extractNuxtData(htmlContent string) (string, error) {
	log.Println("正在提取__NUXT_DATA__内容")

	re := regexp.MustCompile(`<script id="__NUXT_DATA__"[^>]*>(.*?)</script>`)
	matches := re.FindStringSubmatch(htmlContent)

	if len(matches) < 2 {
		log.Println("未找到__NUXT_DATA__标签")
		return "", fmt.Errorf("__NUXT_DATA__ not found")
	}

	log.Println("成功找到__NUXT_DATA__标签")
	return matches[1], nil
}

func (e *V2rayseExtractor) isBase64(s string) bool {
	if len(s)%4 != 0 {
		return false
	}

	base64Pattern := regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)
	return base64Pattern.MatchString(s)
}

func (e *V2rayseExtractor) findBase64Strings(data interface{}, parentKey string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			newKey := fmt.Sprintf("%s.%s", parentKey, key)
			if parentKey == "" {
				newKey = key
			}
			e.findBase64Strings(value, newKey)
		}
	case []interface{}:
		for i, item := range v {
			newKey := fmt.Sprintf("%s[%d]", parentKey, i)
			if parentKey == "" {
				newKey = fmt.Sprintf("[%d]", i)
			}
			e.findBase64Strings(item, newKey)
		}
	case string:
		if len(v) > 20 && e.isBase64(v) {
			e.base64Strings = append(e.base64Strings, v)
		}
	}
}

func (e *V2rayseExtractor) processURL(url string) []string {
	log.Printf("\n%s", strings.Repeat("=", 80))
	log.Printf("开始处理: %s", url)
	log.Println(strings.Repeat("=", 80))

	htmlContent, err := e.fetchHTML(url)
	if err != nil {
		log.Printf("处理 %s 失败: 无法获取HTML内容", url)
		return []string{}
	}

	nuxtDataStr, err := e.extractNuxtData(htmlContent)
	if err != nil {
		log.Printf("未找到__NUXT_DATA__，尝试其他方法提取...")
		return e.tryAlternativeExtraction(htmlContent)
	}

	var nuxtData interface{}
	if err := json.Unmarshal([]byte(nuxtDataStr), &nuxtData); err != nil {
		log.Printf("解析JSON失败: %v", err)
		return []string{}
	}

	log.Println("正在查找base64编码的字符串")
	e.base64Strings = []string{}
	e.findBase64Strings(nuxtData, "")

	log.Printf("找到 %d 个base64编码的字符串", len(e.base64Strings))

	var allLines []string
	for _, base64Str := range e.base64Strings {
		decoded, err := base64.StdEncoding.DecodeString(base64Str)
		if err != nil {
			log.Printf("解码base64字符串失败: %v", err)
			continue
		}

		for _, line := range strings.Split(string(decoded), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				allLines = append(allLines, line)
			}
		}
	}

	log.Printf("完成处理: %s", url)
	return allLines
}

func (e *V2rayseExtractor) tryAlternativeExtraction(htmlContent string) []string {
	log.Println("尝试从HTML中直接查找base64编码的字符串...")
	
	var allLines []string
	
	base64Pattern := regexp.MustCompile(`[A-Za-z0-9+/]{30,}={0,2}`)
	matches := base64Pattern.FindAllString(htmlContent, -1)
	
	log.Printf("找到 %d 个可能的base64字符串", len(matches))
	
	for _, match := range matches {
		if e.isBase64(match) {
			decoded, err := base64.StdEncoding.DecodeString(match)
			if err != nil {
				continue
			}
			
			decodedStr := string(decoded)
			
			if strings.Contains(decodedStr, "ss://") || 
			   strings.Contains(decodedStr, "vmess://") || 
			   strings.Contains(decodedStr, "vless://") || 
			   strings.Contains(decodedStr, "trojan://") {
				
				for _, line := range strings.Split(decodedStr, "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						allLines = append(allLines, line)
					}
				}
			}
		}
	}
	
	log.Printf("备用方法提取到 %d 行", len(allLines))
	return allLines
}

func (e *V2rayseExtractor) Run(urls []string) []string {
	if len(urls) == 0 {
		urls = []string{
			"https://test.v2rayse.com/live-node",
			"https://test.v2rayse.com/free-node",
		}
	}

	log.Println("开始处理多个URL")
	log.Println(strings.Repeat("=", 80))

	var allLines []string
	for _, url := range urls {
		lines := e.processURL(url)
		allLines = append(allLines, lines...)
	}

	log.Println("\n" + strings.Repeat("=", 80))
	log.Println("所有URL处理完成")
	log.Println(strings.Repeat("=", 80))

	uniqueLines := e.deduplicate(allLines)
	log.Printf("\n解码处理完成")
	log.Printf("总计解码: %d 行", len(allLines))
	log.Printf("去重后剩余: %d 行", len(uniqueLines))

	return uniqueLines
}

func (e *V2rayseExtractor) deduplicate(lines []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, line := range lines {
		if !seen[line] {
			seen[line] = true
			result = append(result, line)
		}
	}

	return result
}
