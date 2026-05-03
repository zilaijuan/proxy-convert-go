package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"proxy-convert/internal/config"
	"proxy-convert/internal/database"
	"proxy-convert/internal/handlers"
	"proxy-convert/internal/logger"
	"proxy-convert/internal/scheduler"
	"proxy-convert/internal/service"

	"github.com/gin-gonic/gin"
)

func main() {
	logger.Println("Starting Proxy Convert Server...")

	cfg := config.Load()

	db, err := database.New(cfg.Database.Path)
	if err != nil {
		logger.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	linkService := service.NewLinkService(db)
	verifierService := service.NewVerifierService(db, cfg)
	extractorService := service.NewExtractorService(db, cfg)
	clashService := service.NewClashService(db)

	router := gin.Default()

	handlers.RegisterRoutes(router, linkService, verifierService, extractorService, clashService)

	sched := scheduler.NewScheduler(linkService, verifierService, extractorService, cfg)
	go sched.Start()

	// 创建HTTP服务器
	server := &http.Server{
		Addr:    cfg.Server.Addr,
		Handler: router,
	}

	// 启动服务器
	go func() {
		logger.Printf("Server starting on %s...", cfg.Server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Println("Shutting down server...")

	// 停止scheduler
	sched.Stop()
	logger.Println("Scheduler stopped")

	// 优雅关闭服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Println("Server exiting")
}
