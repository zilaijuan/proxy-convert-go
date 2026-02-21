package scheduler

import (
	"log"
	"proxy-convert/internal/service"
	"time"
)

type Scheduler struct {
	linkService      *service.LinkService
	verifierService  *service.VerifierService
	extractorService *service.ExtractorService
	stopChan         chan struct{}
}

func NewScheduler(linkService *service.LinkService, verifierService *service.VerifierService, extractorService *service.ExtractorService) *Scheduler {
	return &Scheduler{
		linkService:      linkService,
		verifierService:  verifierService,
		extractorService: extractorService,
		stopChan:         make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	log.Println("启动定时任务线程...")

	ticker := time.NewTicker(4 * time.Hour)
	defer ticker.Stop()

	s.runScheduledTasks()

	for {
		select {
		case <-ticker.C:
			s.runScheduledTasks()
		case <-s.stopChan:
			log.Println("定时任务已停止")
			return
		}
	}
}

func (s *Scheduler) Stop() {
	close(s.stopChan)
}

func (s *Scheduler) runScheduledTasks() {
	log.Printf("\n[定时任务] 开始执行 - %s", time.Now().Format("2006-01-02 15:04:05"))

	if err := s.extractorService.ExtractFromV2rayse(); err != nil {
		log.Printf("[定时任务] V2rayseExtractor执行失败: %v", err)
	}

	if err := s.extractorService.ExtractFromGitHub(); err != nil {
		log.Printf("[定时任务] GitHubExtractor执行失败: %v", err)
	}

	log.Println("[定时任务] 开始验证所有节点...")
	if err := s.verifierService.VerifyLinks(); err != nil {
		log.Printf("[定时任务] 节点验证失败: %v", err)
	}

	log.Printf("[定时任务] 执行完成 - %s", time.Now().Format("2006-01-02 15:04:05"))
}
