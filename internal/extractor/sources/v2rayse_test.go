package sources

import "testing"

func TestExtractNuxtDataWithIDAfterOtherAttributes(t *testing.T) {
	html := `<script type="application/json" data-nuxt-data="nuxt-app" id="__NUXT_DATA__">{"ok":true}</script>`

	data, err := extractNuxtData(html)
	if err != nil {
		t.Fatalf("expected nuxt data, got error: %v", err)
	}
	if data != `{"ok":true}` {
		t.Fatalf("unexpected data: %s", data)
	}
}

func TestV2rayseExtractFromAPIResponse(t *testing.T) {
	source := NewV2rayseSource()
	content := `{
		"nodes": [
			{"id": 1, "type": "vless", "uri": "vless://uuid@example.com:443#node"},
			{"id": 2, "type": "ss", "uri": null},
			{"id": 3, "type": "vmess", "uri": ""}
		],
		"access": {"requiresLogin": true, "canCopyRawNode": false, "canConvert": false}
	}`

	links := source.extractFromAPIResponse(content)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d: %#v", len(links), links)
	}
	if links[0] != "vless://uuid@example.com:443#node" {
		t.Fatalf("unexpected link: %s", links[0])
	}
}

func TestV2rayseExtractFromNodesAPIWithIDs(t *testing.T) {
	source := NewV2rayseSource()
	content := `{
		"nodes": [
			{"id": 11, "type": "vless", "uri": null},
			{"id": 22, "type": "vmess", "uri": "vmess://encoded"}
		],
		"access": {"requiresLogin": true, "canCopyRawNode": false, "canConvert": false}
	}`

	links, ids := source.extractFromNodesAPIWithIDs(content)
	if len(ids) != 2 || ids[0] != 11 || ids[1] != 22 {
		t.Fatalf("unexpected ids: %#v", ids)
	}
	if len(links) != 1 || links[0] != "vmess://encoded" {
		t.Fatalf("unexpected links: %#v", links)
	}
}
