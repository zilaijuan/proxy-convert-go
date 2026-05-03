package service

import (
	"context"
	"fmt"
	"time"

	"proxy-convert/internal/config"
	"proxy-convert/internal/database"
	"proxy-convert/internal/extractor"
	"proxy-convert/internal/logger"
	"proxy-convert/internal/parser"

	"proxy-convert/internal/extractor/sources"
)

type ExtractorService struct {
	db      *database.DB
	cfg     *config.Config
	runner  *extractor.Runner
	fetcher extractor.Fetcher
}

type ImportResult struct {
	Imported int
	Existing int
	Failed   int
}

func NewExtractorService(db *database.DB, cfg *config.Config) *ExtractorService {
	fetcher := extractor.NewHTTPFetcher(10 * time.Second)
	sources.SetV2rayseCredentialsProvider(func() (string, string) {
		latestCfg := config.Get()
		if latestCfg == nil {
			return "", ""
		}
		return latestCfg.Extractor.V2rayse.Email, latestCfg.Extractor.V2rayse.Password
	})
	return &ExtractorService{
		db:      db,
		cfg:     cfg,
		runner:  extractor.NewRunner(fetcher),
		fetcher: fetcher,
	}
}

func (s *ExtractorService) importLinks(links []string, logPrefix string) ImportResult {
	result := ImportResult{}

	for _, link := range links {
		proxy, err := parser.ParseLink(link)
		var fingerprint string
		var name string

		if err != nil {
			logger.Printf("解析link失败: %v, Link内容: %s", err, link)
			fingerprint = link
			name = ""
		} else {
			fingerprint = parser.GetNodeFingerprint(proxy)
			name = proxy.Name
		}

		id, err := s.db.AddLink(link, 0, fingerprint, name)
		if err != nil {
			result.Existing++
			continue
		}

		result.Imported++
		if id > 0 {
			logger.Printf("导入link ID: %d", id)
		}
	}

	logger.Printf("%s链接导入完成: 新增 %d 个, 已存在 %d 个", logPrefix, result.Imported, result.Existing)
	return result
}

func (s *ExtractorService) ExtractFromSources(ctx context.Context) (ImportResult, error) {
	results := s.runner.Run(ctx)
	total := ImportResult{}

	for _, sourceResult := range results {
		if sourceResult.Err != nil {
			total.Failed++
			continue
		}

		importResult := s.importLinks(sourceResult.Links, sourceResult.SourceName)
		total.Imported += importResult.Imported
		total.Existing += importResult.Existing
		total.Failed += importResult.Failed
	}

	logger.Printf("所有来源导入完成: 新增 %d 个, 已存在 %d 个, 失败来源 %d 个", total.Imported, total.Existing, total.Failed)
	return total, nil
}

func (s *ExtractorService) ExtractFromAllSources() error {
	_, err := s.ExtractFromSources(context.Background())
	return err
}

func (s *ExtractorService) ExtractFromV2rayse() error {
	_, err := s.ExtractFromSources(context.Background())
	return err
}

func (s *ExtractorService) ExtractFromGitHub() error {
	_, err := s.ExtractFromSources(context.Background())
	return err
}

func (s *ExtractorService) ImportFromURL(url string) (int, int, error) {
	contentParser := extractor.NewContentParser()

	content, err := s.fetcher.Fetch(context.Background(), url)
	if err != nil {
		return 0, 0, fmt.Errorf("获取URL内容失败: %w", err)
	}

	links := contentParser.ParseContent(content)
	result := s.importLinks(links, "")
	return result.Imported, result.Existing, nil
}

func (s *ExtractorService) ImportFromText(text string) (int, int, error) {
	contentParser := extractor.NewContentParser()
	links := contentParser.ParseContent(text)
	result := s.importLinks(links, "")
	return result.Imported, result.Existing, nil
}
