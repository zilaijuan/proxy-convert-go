package extractor

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestContentParserParsesBase64Subscription(t *testing.T) {
	content := "ss://first\nvmess://second\n# comment\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	links := NewContentParser().ParseContent(encoded)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %#v", len(links), links)
	}
	if links[0] != "ss://first" || links[1] != "vmess://second" {
		t.Fatalf("unexpected links: %#v", links)
	}
}

func TestContentParserParsesBase64SubscriptionWithWhitespace(t *testing.T) {
	content := "ss://first\nvmess://second\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	encoded = encoded[:8] + "\n" + encoded[8:] + "\n"

	links := NewContentParser().ParseContent(encoded)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %#v", len(links), links)
	}
}

func TestContentParserParsesRawBase64Subscription(t *testing.T) {
	content := "ss://first\nvmess://second\n"
	encoded := strings.TrimRight(base64.StdEncoding.EncodeToString([]byte(content)), "=")

	links := NewContentParser().ParseContent(encoded)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %#v", len(links), links)
	}
}

func TestContentParserParsesBase64SubscriptionWithTrailingComment(t *testing.T) {
	content := "ss://first\nvmess://second\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(content)) + "#2026-05-03"

	links := NewContentParser().ParseContent(encoded)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %#v", len(links), links)
	}
	if links[0] != "ss://first" || links[1] != "vmess://second" {
		t.Fatalf("unexpected links: %#v", links)
	}
}
