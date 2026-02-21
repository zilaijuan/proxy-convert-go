package main

import (
	"log"
	"proxy-convert/internal/config"
	"proxy-convert/internal/database"
	"proxy-convert/internal/handlers"
	"proxy-convert/internal/scheduler"
	"proxy-convert/internal/service"

	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("Starting Proxy Convert Server...")

	cfg := config.Load()

	db, err := database.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	linkService := service.NewLinkService(db)
	verifierService := service.NewVerifierService(db)
	extractorService := service.NewExtractorService(db)
	clashService := service.NewClashService(db)

	router := gin.Default()

	handlers.RegisterRoutes(router, linkService, verifierService, extractorService, clashService)

	sched := scheduler.NewScheduler(linkService, verifierService, extractorService)
	go sched.Start()

	log.Printf("Server starting on %s...", cfg.Server.Addr)
	if err := router.Run(cfg.Server.Addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
