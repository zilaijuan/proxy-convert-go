package extractor

import (
	"encoding/base64"
	"regexp"
	"strings"
)

type ContentParser struct{}

func NewContentParser() *ContentParser {
	return &ContentParser{}
}

func (p *ContentParser) ParseContent(content string) []string {
	decodedText := p.tryDecodeBase64(content)
	if decodedText != "" {
		content = decodedText
	}

	var links []string
	for _, line := range strings.Split(content, "\n") {
		link := strings.TrimSpace(line)
		if link != "" && !strings.HasPrefix(link, "#") {
			links = append(links, link)
		}
	}

	return links
}

func (p *ContentParser) tryDecodeBase64(text string) string {
	if !p.isLikelyBase64(text) {
		return ""
	}

	decoded, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return ""
	}

	return string(decoded)
}

func (p *ContentParser) isLikelyBase64(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) < 4 {
		return false
	}

	base64Pattern := regexp.MustCompile(`^[A-Za-z0-9+/=_-]+$`)
	return base64Pattern.MatchString(text)
}
