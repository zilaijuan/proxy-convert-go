package extractor

import "context"

type Source interface {
	Type() string
	Name() string
	DefaultURLs() []string
	Extract(ctx context.Context, req SourceRequest) ([]string, error)
}

type SourceRequest struct {
	URLs    []string
	Fetcher Fetcher
}

type Result struct {
	SourceType string
	SourceName string
	Links      []string
	Err        error
}
