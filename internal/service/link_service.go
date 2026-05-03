package service

import (
	"log"
	"proxy-convert/internal/database"
	"proxy-convert/internal/parser"
)

type LinkService struct {
	db *database.DB
}

func NewLinkService(db *database.DB) *LinkService {
	return &LinkService{db: db}
}

func (s *LinkService) AddLink(link string, status int) (int64, error) {
	proxy, err := parser.ParseLink(link)
	if err != nil {
		log.Printf("解析link失败: %v, Link内容: %s", err, link)
		return s.db.AddLink(link, status, "", "")
	}

	fingerprint := parser.GetNodeFingerprint(proxy)
	name := proxy.Name

	return s.db.AddLink(link, status, fingerprint, name)
}

func (s *LinkService) GetAllLinks(statuses []int, limit, offset int) ([]database.Link, error) {
	if limit == 0 {
		limit = 1000
	}
	return s.db.GetAllLinks(statuses, limit, offset)
}

func (s *LinkService) GetLink(id int) (*database.Link, error) {
	return s.db.GetLink(id)
}

func (s *LinkService) UpdateLinkStatus(id int, status int) (bool, error) {
	return s.db.UpdateLink(id, nil, &status, nil)
}

func (s *LinkService) DeleteLink(id int) (bool, error) {
	return s.db.DeleteLink(id)
}

func (s *LinkService) CountLinks(statuses []int) (int, error) {
	return s.db.CountLinks(statuses)
}

func (s *LinkService) DeleteOldUnavailableLinks() (int64, error) {
	return s.db.DeleteOldUnavailableLinks()
}