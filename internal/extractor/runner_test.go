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
