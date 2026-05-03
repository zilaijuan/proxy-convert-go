package scheduler

import (
	"proxy-convert/internal/config"
	"proxy-convert/internal/logger"
	"proxy-convert/internal/service"
	"time"
)

type Scheduler struct {
	linkService      *service.LinkService
	verifierService  *service.VerifierService
	extractorService *service.ExtractorService
	stopChan         chan struct{}
	interval         time.Duration
	lastCleanupDate  string
}

func NewScheduler(linkService *service.LinkService, verifierService *service.VerifierService, extractorService *service.ExtractorService, cfg *config.Config) *Scheduler {
	return &Scheduler{
		linkService:      linkService,
		verifierService:  verifierService,
		extractorService: extractorService,
		stopChan:         make(chan struct{}),
		interval:         cfg.Scheduler.Interval,
	}
}

func (s *Scheduler) Start() {
	logger.Println("启动定时任务线程...")

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.runScheduledTasks()

	for {
		select {
		case <-ticker.C:
			s.runScheduledTasks()
			// 检查并更新间隔时间
			newInterval := config.Get().Scheduler.Interval
			if newInterval != s.interval {
				s.interval = newInterval
				ticker.Reset(s.interval)
				logger.Printf("定时任务间隔已更新为: %v", s.interval)
			}
		case <-s.stopChan:
			logger.Println("定时任务已停止")
			return
		}
	}
}

func (s *Scheduler) Stop() {
	close(s.stopChan)
}

func (s *Scheduler) runScheduledTasks() {
	logger.Printf("\n[定时任务] 开始执行 - %s", time.Now().Format("2006-01-02 15:04:05"))

	if err := s.extractorService.ExtractFromV2rayse(); err != nil {
		logger.Printf("[定时任务] ExtractFromV2rayse执行失败: %v", err)
	}

	if err := s.extractorService.ExtractFromGitHub(); err != nil {
		logger.Printf("[定时任务] ExtractFromGitHub执行失败: %v", err)
	}

	logger.Println("[定时任务] 开始验证所有节点...")
	if err := s.verifierService.VerifyLinks(); err != nil {
		logger.Printf("[定时任务] 节点验证失败: %v", err)
	}

	// 每天执行一次删除半年前status为-1的不可用数据
	currentDate := time.Now().Format("2006-01-02")
	if currentDate != s.lastCleanupDate {
		logger.Println("[定时任务] 开始删除半年前不可用数据...")
		if rowsAffected, err := s.linkService.DeleteOldUnavailableLinks(); err != nil {
			logger.Printf("[定时任务] 删除半年前不可用数据失败: %v", err)
		} else {
			logger.Printf("[定时任务] 成功删除 %d 条半年前不可用数据", rowsAffected)
		}
		// 无论成功失败，都更新日期标记，确保每天只尝试一次
		s.lastCleanupDate = currentDate
	}

	logger.Printf("[定时任务] 执行完成 - %s", time.Now().Format("2006-01-02 15:04:05"))
}
