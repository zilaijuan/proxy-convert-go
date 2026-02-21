package config

import (
	"os"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Verifier VerifierConfig
}

type ServerConfig struct {
	Addr string
}

type DatabaseConfig struct {
	Path string
}

type VerifierConfig struct {
	Timeout      time.Duration
	ChunkSize    int
	TestSpeed    bool
	DownloadSize int64
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Addr: getEnv("SERVER_ADDR", "0.0.0.0:5000"),
		},
		Database: DatabaseConfig{
			Path: getEnv("DATABASE_PATH", "./database/links.db"),
		},
		Verifier: VerifierConfig{
			Timeout:      20 * time.Second,
			ChunkSize:    10,
			TestSpeed:    false,
			DownloadSize: 250000,
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
