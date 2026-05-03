package sources

import (
	"context"
	"encoding/hex"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"proxy-convert/internal/extractor"
	"proxy-convert/internal/logger"
)

var (
	banyunxiaoxiLatestPostPattern = regexp.MustCompile(`<h3 class="title">\s*<a href="([^"]+)"[^>]*>([^<]*节点更新[^<]*)</a>\s*</h3>`)
	banyunxiaoxiCFEmailPattern    = regexp.MustCompile(`(?is)<a[^>]+data-cfemail="([0-9a-fA-F]+)"[^>]*>.*?</a>`)
	banyunxiaoxiTagPattern        = regexp.MustCompile(`(?s)<[^>]+>`)
	banyunxiaoxiProxyPattern      = regexp.MustCompile(`(?i)(ss|vmess|vless|trojan|hysteria2|hy2|anytls)://[^\s<>"']+`)
	banyunxiaoxiTrimRight         = "，。；;、,.!！?？)]}）】》"
	banyunxiaoxiStateMu           sync.RWMutex
	banyunxiaoxiLastFetchedDate   string
	banyunxiaoxiLastFetchedURL    string
)

type BanyunxiaoxiSource struct{}

func NewBanyunxiaoxiSource() *BanyunxiaoxiSource {
	return &BanyunxiaoxiSource{}
}

func (s *BanyunxiaoxiSource) Type() string {
	return "banyunxiaoxi"
}

func (s *BanyunxiaoxiSource) Name() string {
	return "小汐搬运"
}

func (s *BanyunxiaoxiSource) DefaultURLs() []string {
	return []string{"https://blog.banyunxiaoxi.icu/"}
}

func (s *BanyunxiaoxiSource) Extract(ctx context.Context, req extractor.SourceRequest) ([]string, error) {
	today := time.Now().Format("2006-01-02")
	if alreadyFetchedToday(today) {
		logger.Printf("[extractor:%s] today already fetched, skip", s.Type())
		return nil, nil
	}

	if len(req.URLs) == 0 {
		return nil, nil
	}

	listHTML, err := req.Fetcher.Fetch(ctx, req.URLs[0])
	if err != nil {
		return nil, err
	}

	latestURL, title, err := findBanyunxiaoxiLatestPost(listHTML)
	if err != nil {
		return nil, err
	}
	logger.Printf("[extractor:%s] latest post: %s %s", s.Type(), title, latestURL)

	postHTML, err := req.Fetcher.Fetch(ctx, latestURL)
	if err != nil {
		return nil, err
	}

	links := extractBanyunxiaoxiProxyLinks(postHTML)
	if len(links) == 0 {
		return nil, fmt.Errorf("no proxy links found in latest post: %s", latestURL)
	}

	markBanyunxiaoxiFetched(today, latestURL)

	logger.Printf("[extractor:%s] extracted %d links", s.Type(), len(links))
	return links, nil
}

func findBanyunxiaoxiLatestPost(listHTML string) (string, string, error) {
	matches := banyunxiaoxiLatestPostPattern.FindStringSubmatch(listHTML)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("latest post not found")
	}

	postURL := html.UnescapeString(matches[1])
	if _, err := url.ParseRequestURI(postURL); err != nil {
		return "", "", fmt.Errorf("invalid latest post url %q: %w", postURL, err)
	}

	title := strings.TrimSpace(html.UnescapeString(matches[2]))
	return postURL, title, nil
}

func extractBanyunxiaoxiProxyLinks(postHTML string) []string {
	content := decodeCloudflareEmails(postHTML)
	content = strings.ReplaceAll(content, "<br>", "\n")
	content = strings.ReplaceAll(content, "<br/>", "\n")
	content = strings.ReplaceAll(content, "<br />", "\n")
	content = banyunxiaoxiTagPattern.ReplaceAllString(content, "\n")
	content = html.UnescapeString(content)

	matches := banyunxiaoxiProxyPattern.FindAllString(content, -1)
	links := make([]string, 0, len(matches))
	for _, match := range matches {
		link := strings.TrimRight(strings.TrimSpace(match), banyunxiaoxiTrimRight)
		if link != "" {
			links = append(links, link)
		}
	}

	return extractor.Dedupe(links)
}

func decodeCloudflareEmails(content string) string {
	return banyunxiaoxiCFEmailPattern.ReplaceAllStringFunc(content, func(match string) string {
		groups := banyunxiaoxiCFEmailPattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}

		decoded, err := decodeCloudflareEmail(groups[1])
		if err != nil {
			return match
		}
		return decoded
	})
}

func decodeCloudflareEmail(encoded string) (string, error) {
	data, err := hex.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(data) < 2 {
		return "", fmt.Errorf("invalid cfemail length")
	}

	key := data[0]
	decoded := make([]byte, 0, len(data)-1)
	for _, b := range data[1:] {
		decoded = append(decoded, b^key)
	}
	return string(decoded), nil
}

func alreadyFetchedToday(today string) bool {
	banyunxiaoxiStateMu.RLock()
	defer banyunxiaoxiStateMu.RUnlock()
	return banyunxiaoxiLastFetchedDate == today
}

func markBanyunxiaoxiFetched(today, latestURL string) {
	banyunxiaoxiStateMu.Lock()
	defer banyunxiaoxiStateMu.Unlock()
	banyunxiaoxiLastFetchedDate = today
	banyunxiaoxiLastFetchedURL = latestURL
}

func init() {
	extractor.RegisterSource(NewBanyunxiaoxiSource())
}
