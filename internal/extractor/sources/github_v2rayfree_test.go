package sources

import (
	"context"
	"fmt"
	"testing"

	"proxy-convert/internal/extractor"
)

type githubV2rayfreeTestFetcher map[string]string

func (f githubV2rayfreeTestFetcher) Fetch(ctx context.Context, url string) (string, error) {
	content, ok := f[url]
	if !ok {
		return "", fmt.Errorf("unexpected url: %s", url)
	}
	return content, nil
}

func TestGitHubV2rayfreeFindSubscriptionURLFromREADME(t *testing.T) {
	readme := `
older https://raw.githubusercontent.com/free-nodes/v2rayfree/main/v202605031
latest https://raw.githubusercontent.com/free-nodes/v2rayfree/main/v202605032
`

	url := findGithubV2rayfreeSubscriptionURL(readme)
	if url != "https://raw.githubusercontent.com/free-nodes/v2rayfree/main/v202605032" {
		t.Fatalf("unexpected subscription url: %s", url)
	}
}

func TestGitHubV2rayfreeExtractFromREADMESubscription(t *testing.T) {
	source := NewGitHubV2rayfreeSource()
	fetcher := githubV2rayfreeTestFetcher{
		githubV2rayfreeREADMEURL: `
## v2ray免费节点订阅
https://raw.githubusercontent.com/free-nodes/v2rayfree/main/v202605032
`,
		"https://raw.githubusercontent.com/free-nodes/v2rayfree/main/v202605032": "ss://first\nvmess://second\n",
	}

	links, err := source.Extract(context.Background(), extractor.SourceRequest{
		URLs:    source.DefaultURLs(),
		Fetcher: fetcher,
	})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	if len(links) != 2 || links[0] != "ss://first" || links[1] != "vmess://second" {
		t.Fatalf("unexpected links: %#v", links)
	}
}
