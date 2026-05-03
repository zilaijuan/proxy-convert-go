package extractor

import (
	"encoding/base64"
	"regexp"
	"strings"
)

var base64Pattern = regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)

func IsStrictBase64(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 0 || len(s)%4 != 0 {
		return false
	}
	return base64Pattern.MatchString(s)
}

func DecodeBase64Lines(encoded string) ([]string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, line := range strings.Split(string(decoded), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}
