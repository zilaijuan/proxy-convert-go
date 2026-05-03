package extractor

import (
	"sort"
	"sync"
)

var sourceRegistry = struct {
	sync.RWMutex
	sources map[string]Source
}{
	sources: make(map[string]Source),
}

func RegisterSource(source Source) {
	if source == nil || source.Type() == "" {
		return
	}

	sourceRegistry.Lock()
	defer sourceRegistry.Unlock()
	sourceRegistry.sources[source.Type()] = source
}

func GetSource(sourceType string) (Source, bool) {
	sourceRegistry.RLock()
	defer sourceRegistry.RUnlock()
	source, ok := sourceRegistry.sources[sourceType]
	return source, ok
}

func ListSources() []Source {
	sourceRegistry.RLock()
	defer sourceRegistry.RUnlock()

	types := make([]string, 0, len(sourceRegistry.sources))
	for sourceType := range sourceRegistry.sources {
		types = append(types, sourceType)
	}
	sort.Strings(types)

	sources := make([]Source, 0, len(types))
	for _, sourceType := range types {
		sources = append(sources, sourceRegistry.sources[sourceType])
	}
	return sources
}
