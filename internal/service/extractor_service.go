package service

import (
	"fmt"
	"io"
	"net/http"
	"proxy-convert/internal/config"
	"proxy-convert/internal/database"
	"proxy-convert/internal/extractor"
	"proxy-convert/internal/logger"
	"proxy-convert/internal/parser"
)

type ExtractorService struct {
	db  *database.DB
	cfg *config.Config
}

func NewExtractorService(db *database.DB, cfg *config.Config) *ExtractorService {
	return &ExtractorService{db: db, cfg: cfg}
}

func (s *ExtractorService) importLinks(links []string, logPrefix string) (int, int) {
	importedCount := 0
	existingCount := 0

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
			existingCount++
		} else {
			importedCount++
			if id > 0 {
				logger.Printf("导入link ID: %d", id)
			}
		}
	}

	logger.Printf("%s链接导入完成: 新增 %d 个, 已存在 %d 个", logPrefix, importedCount, existingCount)
	return importedCount, existingCount
}

func (s *ExtractorService) ExtractFromV2rayse() error {
	// 获取最新配置
	latestCfg := config.Get()
	
	extractor := extractor.NewV2rayseExtractor()
	links := extractor.Run(latestCfg.Extractor.V2rayseURLs)
	s.importLinks(links, "")
	return nil
}

func (s *ExtractorService) ExtractFromGitHub() error {
	// 获取最新配置
	latestCfg := config.Get()
	
	extractor := extractor.NewGitHubExtractor()
	links := extractor.Run(latestCfg.Extractor.GitHubURLs)
	s.importLinks(links, "GitHub")
	return nil
}

func (s *ExtractorService) ImportFromURL(url string) (int, int, error) {
	contentParser := extractor.NewContentParser()

	client := &http.Client{}
	resp, err := client.Get(url)
	if err != nil {
		return 0, 0, fmt.Errorf("获取URL内容失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("读取响应内容失败: %w", err)
	}

	links := contentParser.ParseContent(string(body))
	imported, existing := s.importLinks(links, "")
	return imported, existing, nil
}

func (s *ExtractorService) ImportFromText(text string) (int, int, error) {
	contentParser := extractor.NewContentParser()
	links := contentParser.ParseContent(text)
	imported, existing := s.importLinks(links, "")
	return imported, existing, nil
}
