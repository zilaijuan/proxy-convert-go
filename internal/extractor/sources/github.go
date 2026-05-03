package sources

import (
	"context"

	"proxy-convert/internal/extractor"
	"proxy-convert/internal/logger"
)

type GitHubSource struct{}

func NewGitHubSource() *GitHubSource {
	return &GitHubSource{}
}

func (s *GitHubSource) Type() string {
	return "github"
}

func (s *GitHubSource) Name() string {
	return "GitHub"
}

func (s *GitHubSource) DefaultURLs() []string {
	return []string{
		"https://cdn.jsdmirror.com/gh/arshiacomplus/v2rayExtractor/mix/sub.html",
	}
}

func (s *GitHubSource) Extract(ctx context.Context, req extractor.SourceRequest) ([]string, error) {
	parser := extractor.NewContentParser()
	var allLinks []string

	for _, url := range req.URLs {
		logger.Printf("[extractor:%s] fetching %s", s.Type(), url)
		content, err := req.Fetcher.Fetch(ctx, url)
		if err != nil {
			logger.Printf("[extractor:%s] fetch failed: %s: %v", s.Type(), url, err)
			continue
		}

		allLinks = append(allLinks, parser.ParseContent(content)...)
	}

	return extractor.Dedupe(allLinks), nil
}

func init() {
	extractor.RegisterSource(NewGitHubSource())
}
