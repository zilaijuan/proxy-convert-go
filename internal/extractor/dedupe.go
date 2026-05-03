package extractor

func Dedupe(lines []string) []string {
	seen := make(map[string]struct{}, len(lines))
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		result = append(result, line)
	}

	return result
}
