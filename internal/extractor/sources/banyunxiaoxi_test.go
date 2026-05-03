package sources

import "testing"

func TestFindBanyunxiaoxiLatestPost(t *testing.T) {
	listHTML := `<div class="a-post 1">
		<div class="header">
			<h3 class="title"><a href="https://blog.banyunxiaoxi.icu/2026/05/02/latest/">2026-05-02节点更新 - 可用节点数117个</a></h3>
		</div>
	</div>`

	url, title, err := findBanyunxiaoxiLatestPost(listHTML)
	if err != nil {
		t.Fatalf("expected latest post, got error: %v", err)
	}

	if url != "https://blog.banyunxiaoxi.icu/2026/05/02/latest/" {
		t.Fatalf("unexpected url: %s", url)
	}
	if title != "2026-05-02节点更新 - 可用节点数117个" {
		t.Fatalf("unexpected title: %s", title)
	}
}

func TestExtractBanyunxiaoxiProxyLinksDecodesCloudflareEmail(t *testing.T) {
	postHTML := `ss://<a href="/cdn-cgi/l/email-protection" class="__cf_email__" data-cfemail="10715072">[email&#160;protected]</a>:989#node
		<br>vmess://encoded-payload`

	links := extractBanyunxiaoxiProxyLinks(postHTML)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %#v", len(links), links)
	}

	if links[0] != "ss://a@b:989#node" {
		t.Fatalf("unexpected ss link: %s", links[0])
	}
	if links[1] != "vmess://encoded-payload" {
		t.Fatalf("unexpected vmess link: %s", links[1])
	}
}
