package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"proxy-convert/internal/extractor"
	"proxy-convert/internal/logger"
)

const (
	v2rayseBaseURL       = "https://v2rayse.com"
	v2rayseLoginURL      = v2rayseBaseURL + "/auth/login"
	v2rayseFreeNodeURL   = v2rayseBaseURL + "/free-node"
	v2rayseNodesAPIURL   = v2rayseBaseURL + "/api/tools/free-node-share/nodes"
	v2rayseCheckInAPIURL = v2rayseBaseURL + "/api/account/points/check-in"
	v2rayseConvertAPIURL = v2rayseBaseURL + "/api/tools/free-node-share/convert"
	v2rayseStatePath     = "database/source_state/v2rayse.json"
)

var v2rayseCredentialsProvider func() (string, string)

var (
	v2rayseDailyStateMu        sync.RWMutex
	v2rayseLastSuccessfulDate  string
	v2rayseLastSuccessfulCount int
	v2rayseLastCheckInDate     string
)

type v2rayseState struct {
	LastSuccessfulDate  string `json:"last_successful_date"`
	LastSuccessfulCount int    `json:"last_successful_count"`
	LastCheckInDate     string `json:"last_check_in_date"`
}

type V2rayseSource struct{}

func NewV2rayseSource() *V2rayseSource {
	return &V2rayseSource{}
}

func (s *V2rayseSource) Type() string {
	return "v2rayse"
}

func (s *V2rayseSource) Name() string {
	return "V2rayse"
}

func (s *V2rayseSource) DefaultURLs() []string {
	return []string{
		v2rayseNodesAPIURL,
	}
}

func (s *V2rayseSource) Extract(ctx context.Context, req extractor.SourceRequest) ([]string, error) {
	today := time.Now().Format("2006-01-02")
	if ok, count := v2rayseAlreadyExtractedToday(today); ok {
		logger.Printf("[extractor:%s] today already extracted %d links, skip", s.Type(), count)
		return nil, nil
	}

	if links, ok := s.extractAuthenticated(ctx); ok {
		if len(links) > 0 {
			markV2rayseExtracted(today, len(links))
		}
		return links, nil
	}

	var allLines []string

	for _, url := range req.URLs {
		logger.Printf("[extractor:%s] fetching %s", s.Type(), url)
		htmlContent, err := req.Fetcher.Fetch(ctx, url)
		if err != nil {
			logger.Printf("[extractor:%s] fetch failed: %s: %v", s.Type(), url, err)
			continue
		}

		allLines = append(allLines, s.extractFromContent(htmlContent)...)
	}

	allLines = extractor.Dedupe(allLines)
	if len(allLines) > 0 {
		markV2rayseExtracted(today, len(allLines))
	}
	return allLines, nil
}

func v2rayseAlreadyExtractedToday(today string) (bool, int) {
	v2rayseDailyStateMu.RLock()
	if v2rayseLastSuccessfulDate == today {
		count := v2rayseLastSuccessfulCount
		v2rayseDailyStateMu.RUnlock()
		return true, count
	}
	v2rayseDailyStateMu.RUnlock()

	state := loadV2rayseState()
	return state.LastSuccessfulDate == today, state.LastSuccessfulCount
}

func markV2rayseExtracted(today string, count int) {
	v2rayseDailyStateMu.Lock()
	v2rayseLastSuccessfulDate = today
	v2rayseLastSuccessfulCount = count
	v2rayseDailyStateMu.Unlock()

	state := loadV2rayseState()
	state.LastSuccessfulDate = today
	state.LastSuccessfulCount = count
	saveV2rayseState(state)
}

func v2rayseAlreadyCheckedInToday(today string) bool {
	v2rayseDailyStateMu.RLock()
	if v2rayseLastCheckInDate == today {
		v2rayseDailyStateMu.RUnlock()
		return true
	}
	v2rayseDailyStateMu.RUnlock()

	state := loadV2rayseState()
	return state.LastCheckInDate == today
}

func markV2rayseCheckedIn(today string) {
	v2rayseDailyStateMu.Lock()
	v2rayseLastCheckInDate = today
	v2rayseDailyStateMu.Unlock()

	state := loadV2rayseState()
	state.LastCheckInDate = today
	saveV2rayseState(state)
}

func loadV2rayseState() v2rayseState {
	v2rayseDailyStateMu.Lock()
	defer v2rayseDailyStateMu.Unlock()

	data, err := os.ReadFile(v2rayseStatePath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Printf("[extractor:v2rayse] load state failed: %v", err)
		}
		return v2rayseState{
			LastSuccessfulDate:  v2rayseLastSuccessfulDate,
			LastSuccessfulCount: v2rayseLastSuccessfulCount,
			LastCheckInDate:     v2rayseLastCheckInDate,
		}
	}

	var state v2rayseState
	if err := json.Unmarshal(data, &state); err != nil {
		logger.Printf("[extractor:v2rayse] parse state failed: %v", err)
		return v2rayseState{}
	}

	v2rayseLastSuccessfulDate = state.LastSuccessfulDate
	v2rayseLastSuccessfulCount = state.LastSuccessfulCount
	v2rayseLastCheckInDate = state.LastCheckInDate
	return state
}

func saveV2rayseState(state v2rayseState) {
	v2rayseDailyStateMu.Lock()
	defer v2rayseDailyStateMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(v2rayseStatePath), 0755); err != nil {
		logger.Printf("[extractor:v2rayse] create state dir failed: %v", err)
		return
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logger.Printf("[extractor:v2rayse] encode state failed: %v", err)
		return
	}

	if err := os.WriteFile(v2rayseStatePath, data, 0644); err != nil {
		logger.Printf("[extractor:v2rayse] save state failed: %v", err)
		return
	}

	v2rayseLastSuccessfulDate = state.LastSuccessfulDate
	v2rayseLastSuccessfulCount = state.LastSuccessfulCount
	v2rayseLastCheckInDate = state.LastCheckInDate
}

func (s *V2rayseSource) extractAuthenticated(ctx context.Context) ([]string, bool) {
	email, password := getV2rayseCredentials()
	if email == "" || password == "" {
		logger.Printf("[extractor:%s] v2rayse email/password not configured, using anonymous mode", s.Type())
		return nil, false
	}

	client, err := newV2rayseClient()
	if err != nil {
		logger.Printf("[extractor:%s] create auth client failed: %v", s.Type(), err)
		return nil, false
	}

	if err := client.login(ctx, email, password); err != nil {
		logger.Printf("[extractor:%s] login failed: %v", s.Type(), err)
		return nil, false
	}

	s.checkInOncePerDay(ctx, client)

	nodesContent, err := client.get(ctx, v2rayseNodesAPIURL, "application/json, text/plain, */*", v2rayseFreeNodeURL)
	if err != nil {
		logger.Printf("[extractor:%s] fetch authenticated nodes failed: %v", s.Type(), err)
	} else {
		links, ids := s.extractFromNodesAPIWithIDs(nodesContent)
		if len(links) > 0 {
			return links, true
		}

		converted, err := client.convert(ctx, ids)
		if err != nil {
			logger.Printf("[extractor:%s] convert selected nodes failed: %v", s.Type(), err)
		} else if links := splitProxyLinks(converted.Content); len(links) > 0 {
			logger.Printf("[extractor:%s] converted %d selected nodes into %d links", s.Type(), converted.SelectedCount, len(links))
			return links, true
		}
	}

	pageContent, err := client.get(ctx, v2rayseFreeNodeURL, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", v2rayseBaseURL+"/user/points")
	if err != nil {
		logger.Printf("[extractor:%s] fetch authenticated free-node page failed: %v", s.Type(), err)
		return nil, true
	}

	return s.extractFromHTML(pageContent), true
}

func (s *V2rayseSource) checkInOncePerDay(ctx context.Context, client *v2rayseClient) {
	today := time.Now().Format("2006-01-02")
	if v2rayseAlreadyCheckedInToday(today) {
		logger.Printf("[extractor:%s] check-in already handled today, skip", s.Type())
		return
	}

	checkInStatus, err := client.checkIn(ctx)
	if err != nil {
		logger.Printf("[extractor:%s] check-in skipped/failed: %v", s.Type(), err)
		return
	}

	markV2rayseCheckedIn(today)
	logger.Printf("[extractor:%s] check-in %s", s.Type(), checkInStatus)
}

func SetV2rayseCredentialsProvider(provider func() (string, string)) {
	v2rayseCredentialsProvider = provider
}

func getV2rayseCredentials() (string, string) {
	if v2rayseCredentialsProvider == nil {
		return "", ""
	}
	email, password := v2rayseCredentialsProvider()
	return strings.TrimSpace(email), password
}

func (s *V2rayseSource) extractFromContent(content string) []string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "{") {
		return s.extractFromAPIResponse(content)
	}
	return s.extractFromHTML(content)
}

type v2rayseNodesResponse struct {
	Nodes  []v2rayseNode `json:"nodes"`
	Access struct {
		RequiresLogin  bool `json:"requiresLogin"`
		CanCopyRawNode bool `json:"canCopyRawNode"`
		CanConvert     bool `json:"canConvert"`
	} `json:"access"`
}

type v2rayseNode struct {
	ID   int     `json:"id"`
	URI  *string `json:"uri"`
	Type string  `json:"type"`
}

func (s *V2rayseSource) extractFromAPIResponse(content string) []string {
	links, _ := s.extractFromNodesAPIWithIDs(content)
	return links
}

func (s *V2rayseSource) extractFromNodesAPIWithIDs(content string) ([]string, []int) {
	var response v2rayseNodesResponse
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		logger.Printf("[extractor:%s] parse nodes api failed: %v", s.Type(), err)
		return nil, nil
	}

	var links []string
	var ids []int
	hiddenCount := 0
	for _, node := range response.Nodes {
		if node.ID > 0 {
			ids = append(ids, node.ID)
		}
		if node.URI == nil || strings.TrimSpace(*node.URI) == "" {
			hiddenCount++
			continue
		}
		links = append(links, strings.TrimSpace(*node.URI))
	}

	if len(links) == 0 && hiddenCount > 0 {
		logger.Printf(
			"[extractor:%s] nodes api returned %d nodes but raw uri is hidden (requiresLogin=%v, canCopyRawNode=%v, canConvert=%v)",
			s.Type(),
			hiddenCount,
			response.Access.RequiresLogin,
			response.Access.CanCopyRawNode,
			response.Access.CanConvert,
		)
	}

	return extractor.Dedupe(links), ids
}

func (s *V2rayseSource) extractFromHTML(htmlContent string) []string {
	nuxtData, err := extractNuxtData(htmlContent)
	if err != nil {
		logger.Printf("[extractor:%s] __NUXT_DATA__ not found, trying fallback", s.Type())
		return s.tryAlternativeExtraction(htmlContent)
	}

	var data interface{}
	if err := json.Unmarshal([]byte(nuxtData), &data); err != nil {
		logger.Printf("[extractor:%s] parse __NUXT_DATA__ failed: %v", s.Type(), err)
		return nil
	}

	base64Strings := findBase64Strings(data)
	var allLines []string

	for _, encoded := range base64Strings {
		lines, err := extractor.DecodeBase64Lines(encoded)
		if err != nil {
			logger.Printf("[extractor:%s] decode base64 failed: %v", s.Type(), err)
			continue
		}
		allLines = append(allLines, lines...)
	}

	allLines = append(allLines, findProxyLinks(data)...)
	if len(allLines) == 0 {
		logger.Printf("[extractor:%s] no public raw uri found in nuxt data", s.Type())
	}

	return extractor.Dedupe(allLines)
}

func extractNuxtData(htmlContent string) (string, error) {
	re := regexp.MustCompile(`(?s)<script[^>]*id="__NUXT_DATA__"[^>]*>(.*?)</script>`)
	matches := re.FindStringSubmatch(htmlContent)
	if len(matches) < 2 {
		return "", fmt.Errorf("__NUXT_DATA__ not found")
	}
	return html.UnescapeString(matches[1]), nil
}

func findBase64Strings(data interface{}) []string {
	var result []string
	walkBase64Strings(data, &result)
	return result
}

func walkBase64Strings(data interface{}, result *[]string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for _, value := range v {
			walkBase64Strings(value, result)
		}
	case []interface{}:
		for _, item := range v {
			walkBase64Strings(item, result)
		}
	case string:
		if len(v) > 20 && extractor.IsStrictBase64(v) {
			*result = append(*result, v)
		}
	}
}

func findProxyLinks(data interface{}) []string {
	var result []string
	walkProxyLinks(data, &result)
	return result
}

func walkProxyLinks(data interface{}, result *[]string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for _, value := range v {
			walkProxyLinks(value, result)
		}
	case []interface{}:
		for _, item := range v {
			walkProxyLinks(item, result)
		}
	case string:
		if containsProxyLink(v) {
			*result = append(*result, splitProxyLinks(v)...)
		}
	}
}

func (s *V2rayseSource) tryAlternativeExtraction(htmlContent string) []string {
	base64Pattern := regexp.MustCompile(`[A-Za-z0-9+/]{30,}={0,2}`)
	matches := base64Pattern.FindAllString(htmlContent, -1)
	var allLines []string

	for _, match := range matches {
		if !extractor.IsStrictBase64(match) {
			continue
		}

		lines, err := extractor.DecodeBase64Lines(match)
		if err != nil {
			continue
		}

		decodedText := strings.Join(lines, "\n")
		if !containsProxyLink(decodedText) {
			continue
		}

		allLines = append(allLines, lines...)
	}

	logger.Printf("[extractor:%s] fallback extracted %d links", s.Type(), len(allLines))
	return allLines
}

func splitProxyLinks(text string) []string {
	var links []string
	for _, line := range strings.Fields(text) {
		line = strings.TrimSpace(line)
		if containsProxyLink(line) {
			links = append(links, strings.TrimRight(line, "，。；;、,.!！?？)]}）】》"))
		}
	}
	return extractor.Dedupe(links)
}

func containsProxyLink(text string) bool {
	return strings.Contains(text, "ss://") ||
		strings.Contains(text, "vmess://") ||
		strings.Contains(text, "vless://") ||
		strings.Contains(text, "trojan://") ||
		strings.Contains(text, "hysteria2://") ||
		strings.Contains(text, "hy2://") ||
		strings.Contains(text, "anytls://")
}

type v2rayseClient struct {
	client *http.Client
}

type v2rayseConvertResponse struct {
	Content        string   `json:"content"`
	SelectedCount  int      `json:"selectedCount"`
	ConvertedCount int      `json:"convertedCount"`
	ParsedCount    int      `json:"parsedCount"`
	Warnings       []string `json:"warnings"`
	Charged        int      `json:"charged"`
	Points         *int     `json:"points"`
}

func newV2rayseClient() (*v2rayseClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &v2rayseClient{
		client: &http.Client{
			Timeout: 20 * time.Second,
			Jar:     jar,
		},
	}, nil
}

func (c *v2rayseClient) login(ctx context.Context, email, password string) error {
	if _, err := c.get(ctx, v2rayseLoginURL, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", v2rayseLoginURL); err != nil {
		return err
	}

	form := url.Values{}
	form.Set("email", email)
	form.Set("password", password)
	form.Set("remember", "true")
	form.Set("redirect", "/account")
	form.Set("loginPath", "/auth/login")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v2rayseLoginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	setV2rayseHeaders(req, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", v2rayseLoginURL)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", v2rayseBaseURL)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("login status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func (c *v2rayseClient) checkIn(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v2rayseCheckInAPIURL, nil)
	if err != nil {
		return "", err
	}
	setV2rayseHeaders(req, "*/*", v2rayseBaseURL+"/user/points")
	req.Header.Set("Origin", v2rayseBaseURL)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	bodyText := strings.TrimSpace(string(body))

	if resp.StatusCode >= http.StatusBadRequest {
		if isV2rayseAlreadyCheckedIn(bodyText) {
			return "already completed today", nil
		}
		return "", fmt.Errorf("check-in status %d: %s", resp.StatusCode, bodyText)
	}

	if isV2rayseAlreadyCheckedIn(bodyText) {
		return "already completed today", nil
	}
	if bodyText == "" {
		return "succeeded", nil
	}
	return "succeeded: " + summarizeV2rayseResponse(bodyText), nil
}

func (c *v2rayseClient) convert(ctx context.Context, ids []int) (v2rayseConvertResponse, error) {
	if len(ids) == 0 {
		return v2rayseConvertResponse{}, errors.New("no node ids to convert")
	}

	payload, err := json.Marshal(map[string]interface{}{
		"ids":    ids,
		"target": "v2ray",
	})
	if err != nil {
		return v2rayseConvertResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v2rayseConvertAPIURL, bytes.NewReader(payload))
	if err != nil {
		return v2rayseConvertResponse{}, err
	}
	setV2rayseHeaders(req, "application/json, text/plain, */*", v2rayseFreeNodeURL)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", v2rayseBaseURL)

	resp, err := c.client.Do(req)
	if err != nil {
		return v2rayseConvertResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return v2rayseConvertResponse{}, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return v2rayseConvertResponse{}, fmt.Errorf("convert status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result v2rayseConvertResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return v2rayseConvertResponse{}, err
	}
	return result, nil
}

func (c *v2rayseClient) get(ctx context.Context, url, accept, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	setV2rayseHeaders(req, accept, referer)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("get %s status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return string(body), nil
}

func setV2rayseHeaders(req *http.Request, accept, referer string) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36")
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

func isV2rayseAlreadyCheckedIn(body string) bool {
	body = strings.ToLower(body)
	return strings.Contains(body, "already") ||
		strings.Contains(body, "checked") ||
		strings.Contains(body, "sign") ||
		strings.Contains(body, "今日") ||
		strings.Contains(body, "已经") ||
		strings.Contains(body, "已签到") ||
		strings.Contains(body, "重复")
}

func summarizeV2rayseResponse(body string) string {
	body = strings.ReplaceAll(body, "\n", " ")
	body = strings.ReplaceAll(body, "\r", " ")
	body = strings.Join(strings.Fields(body), " ")
	if len(body) > 180 {
		return body[:180] + "..."
	}
	return body
}

func init() {
	extractor.RegisterSource(NewV2rayseSource())
}
