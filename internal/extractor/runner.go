package extractor

import (
	"context"
	"time"

	"proxy-convert/internal/logger"
)

type Runner struct {
	fetcher Fetcher
}

func NewRunner(fetcher Fetcher) *Runner {
	if fetcher == nil {
		fetcher = NewHTTPFetcher(10 * time.Second)
	}
	return &Runner{fetcher: fetcher}
}

func (r *Runner) Run(ctx context.Context) []Result {
	sources := ListSources()
	results := make([]Result, 0, len(sources))

	for _, source := range sources {
		logger.Printf("[extractor] start source: %s (%s)", source.Name(), source.Type())

		links, err := source.Extract(ctx, SourceRequest{
			URLs:    source.DefaultURLs(),
			Fetcher: r.fetcher,
		})
		if err != nil {
			logger.Printf("[extractor] source failed: %s (%s): %v", source.Name(), source.Type(), err)
			results = append(results, Result{
				SourceType: source.Type(),
				SourceName: source.Name(),
				Err:        err,
			})
			continue
		}

		links = Dedupe(links)
		logger.Printf("[extractor] source finished: %s (%s), links=%d", source.Name(), source.Type(), len(links))
		results = append(results, Result{
			SourceType: source.Type(),
			SourceName: source.Name(),
			Links:      links,
		})
	}

	return results
}

func (r *Runner) RunAll(ctx context.Context) []string {
	results := r.Run(ctx)
	var allLinks []string

	for _, result := range results {
		if result.Err != nil {
			continue
		}
		allLinks = append(allLinks, result.Links...)
	}

	return Dedupe(allLinks)
}
