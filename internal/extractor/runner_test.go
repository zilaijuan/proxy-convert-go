package extractor

import (
	"context"
	"testing"
)

type testSource struct{}

func (s testSource) Type() string {
	return "test-source"
}

func (s testSource) Name() string {
	return "Test Source"
}

func (s testSource) DefaultURLs() []string {
	return []string{"https://example.test/sub"}
}

func (s testSource) Extract(ctx context.Context, req SourceRequest) ([]string, error) {
	content, err := req.Fetcher.Fetch(ctx, req.URLs[0])
	if err != nil {
		return nil, err
	}
	return NewContentParser().ParseContent(content), nil
}

type testFetcher struct {
	content string
}

func (f testFetcher) Fetch(ctx context.Context, url string) (string, error) {
	return f.content, nil
}

type urlContentFetcher map[string]string

func (f urlContentFetcher) Fetch(ctx context.Context, url string) (string, error) {
	return f[url], nil
}

func TestRunnerUsesRegisteredSources(t *testing.T) {
	RegisterSource(testSource{})

	runner := NewRunner(testFetcher{
		content: "ss://first\n# comment\nss://first\nvmess://second",
	})

	links := runner.RunAll(context.Background())
	if len(links) != 2 {
		t.Fatalf("expected 2 deduped links, got %d: %#v", len(links), links)
	}

	if links[0] != "ss://first" {
		t.Fatalf("expected first link to be ss://first, got %q", links[0])
	}
	if links[1] != "vmess://second" {
		t.Fatalf("expected second link to be vmess://second, got %q", links[1])
	}
}

func TestRunnerUsesURLProvider(t *testing.T) {
	source := testSource{}
	RegisterSource(source)

	runner := NewRunnerWithURLProvider(urlContentFetcher{
		"https://example.test/sub":         "ss://from-default",
		"https://example.test/from-config": "ss://from-config",
	}, func(requested Source) []string {
		if requested.Type() != source.Type() {
			return nil
		}
		return []string{"https://example.test/from-config"}
	})

	results := runner.Run(context.Background())
	var found bool
	for _, result := range results {
		if result.SourceType != source.Type() {
			continue
		}
		found = true
		if len(result.Links) != 1 || result.Links[0] != "ss://from-config" {
			t.Fatalf("expected configured url content to be parsed, got %#v", result.Links)
		}
	}

	if !found {
		t.Fatalf("expected test source result")
	}
}
