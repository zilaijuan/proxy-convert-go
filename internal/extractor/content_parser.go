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
	normalized := strings.Join(strings.Fields(text), "")
	if beforeComment, _, found := strings.Cut(normalized, "#"); found {
		normalized = beforeComment
	}
	if !p.isLikelyBase64(normalized) {
		return ""
	}

	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(normalized)
		if err == nil {
			return string(decoded)
		}
	}

	return ""
}

func (p *ContentParser) isLikelyBase64(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) < 4 {
		return false
	}

	base64Pattern := regexp.MustCompile(`^[A-Za-z0-9+/=_-]+$`)
	return base64Pattern.MatchString(text)
}
