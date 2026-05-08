package sources

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"proxy-convert/internal/extractor"
	"proxy-convert/internal/logger"
)

const (
	githubV2rayfreeREADMEURL = "https://ghproxy.net/https://raw.githubusercontent.com/free-nodes/v2rayfree/main/README.md"
)

var (
	githubV2rayfreeRawSubPattern = regexp.MustCompile(`https://raw\.githubusercontent\.com/free-nodes/v2rayfree/main/v\d+`)
)

type GitHubV2rayfreeSource struct{}

func NewGitHubV2rayfreeSource() *GitHubV2rayfreeSource {
	return &GitHubV2rayfreeSource{}
}

func (s *GitHubV2rayfreeSource) Type() string {
	return "github_v2rayfree"
}

func (s *GitHubV2rayfreeSource) Name() string {
	return "GitHub v2rayfree"
}

func (s *GitHubV2rayfreeSource) DefaultURLs() []string {
	return []string{githubV2rayfreeREADMEURL}
}

func (s *GitHubV2rayfreeSource) Extract(ctx context.Context, req extractor.SourceRequest) ([]string, error) {
	subscriptionURL, err := s.findSubscriptionURL(ctx, req)
	if err != nil {
		return nil, err
	}

	logger.Printf("[extractor:%s] fetching subscription %s", s.Type(), subscriptionURL)
	content, err := req.Fetcher.Fetch(ctx, githubV2rayfreeProxyURL(subscriptionURL))
	if err != nil {
		return nil, err
	}

	links := extractor.NewContentParser().ParseContent(content)
	logger.Printf("[extractor:%s] extracted %d links", s.Type(), len(links))
	return extractor.Dedupe(links), nil
}

func githubV2rayfreeProxyURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://ghproxy.net/") {
		return rawURL
	}
	return "https://ghproxy.net/" + rawURL
}


func (s *GitHubV2rayfreeSource) findSubscriptionURL(ctx context.Context, req extractor.SourceRequest) (string, error) {
	readmeURLs := req.URLs
	if len(readmeURLs) == 0 {
		readmeURLs = s.DefaultURLs()
	}

	for _, readmeURL := range readmeURLs {
		logger.Printf("[extractor:%s] fetching README %s", s.Type(), readmeURL)
		readme, err := req.Fetcher.Fetch(ctx, readmeURL)
		if err != nil {
			logger.Printf("[extractor:%s] README fetch failed: %s: %v", s.Type(), readmeURL, err)
			continue
		}

		if subscriptionURL := findGithubV2rayfreeSubscriptionURL(readme); subscriptionURL != "" {
			return subscriptionURL, nil
		}
	}

	return "", fmt.Errorf("subscription URL not found in README")
}

func findGithubV2rayfreeSubscriptionURL(readme string) string {
	matches := githubV2rayfreeRawSubPattern.FindAllString(readme, -1)
	if len(matches) == 0 {
		return ""
	}

	sort.Strings(matches)
	return matches[len(matches)-1]
}

func init() {
	extractor.RegisterSource(NewGitHubV2rayfreeSource())
}
