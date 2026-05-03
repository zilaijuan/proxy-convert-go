package config

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultWindowsMihomoPath = "./bin/mihomo-windows-amd64-v3-go125.exe"
	defaultLinuxMihomoPath   = "./bin/mihomo-linux-amd64-v3-go125"
	defaultDarwinMihomoPath  = "./bin/mihomo-darwin-amd64-v3-go125"
	configPath               = "config.yaml"
	watchInterval            = 2 * time.Second
)

var (
	globalConfig *Config
	configMutex  sync.RWMutex
	lastModified time.Time
	watcherDone  chan struct{}
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Verifier  VerifierConfig  `yaml:"verifier"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	Extractor ExtractorConfig `yaml:"extractor"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type VerifierConfig struct {
	Timeout      time.Duration `yaml:"timeout"`
	ChunkSize    int           `yaml:"chunkSize"`
	TestSpeed    bool          `yaml:"testSpeed"`
	DownloadSize int64         `yaml:"downloadSize"`
	MihomoPath   string        `yaml:"mihomoPath"`
	TestURL      string        `yaml:"testURL"`
	MaxFailCount int           `yaml:"maxFailCount"`
}

type SchedulerConfig struct {
	Interval time.Duration `yaml:"interval"`
}

type ExtractorConfig struct {
	V2rayseURLs []string             `yaml:"v2rayseURLs"`
	GitHubURLs  []string             `yaml:"githubURLs"`
	V2rayse     V2rayseExtractorAuth `yaml:"v2rayse"`
}

type V2rayseExtractorAuth struct {
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
}

func Load() *Config {
	configMutex.Lock()
	defer configMutex.Unlock()

	if globalConfig == nil {
		globalConfig = &Config{
			Server: ServerConfig{
				Addr: "0.0.0.0:5000",
			},
			Database: DatabaseConfig{
				Path: "./database/links.db",
			},
			Verifier: VerifierConfig{
				Timeout:      20 * time.Second,
				ChunkSize:    10,
				TestSpeed:    false,
				DownloadSize: 250000,
				MihomoPath:   defaultMihomoPath(),
				TestURL:      "https://www.google.com/generate_204",
				MaxFailCount: 10,
			},
			Scheduler: SchedulerConfig{
				Interval: 4 * time.Hour,
			},
			Extractor: ExtractorConfig{
				V2rayseURLs: []string{
					"https://test.v2rayse.com/live-node",
					"https://test.v2rayse.com/free-node",
				},
				GitHubURLs: []string{
					"https://cdn.jsdmirror.com/gh/arshiacomplus/v2rayExtractor/mix/sub.html",
				},
			},
		}

		loadFromConfigFile(globalConfig)
		applyEnvOverrides(globalConfig)

		// 启动配置文件监控
		startConfigWatcher()
	}

	return globalConfig
}

// Get 获取当前配置
func Get() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}

// startConfigWatcher 启动配置文件监控
func startConfigWatcher() {
	watcherDone = make(chan struct{})

	go func() {
		ticker := time.NewTicker(watchInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				checkConfigChanges()
			case <-watcherDone:
				return
			}
		}
	}()
}

// checkConfigChanges 检查配置文件是否变化
func checkConfigChanges() {
	info, err := os.Stat(configPath)
	if err != nil {
		return
	}

	if info.ModTime().After(lastModified) {
		lastModified = info.ModTime()
		reloadConfig()
	}
}

// reloadConfig 重新加载配置
func reloadConfig() {
	configMutex.Lock()
	defer configMutex.Unlock()

	// 创建新的配置实例
	newConfig := &Config{
		Server: ServerConfig{
			Addr: "0.0.0.0:5000",
		},
		Database: DatabaseConfig{
			Path: "./database/links.db",
		},
		Verifier: VerifierConfig{
			Timeout:      20 * time.Second,
			ChunkSize:    10,
			TestSpeed:    false,
			DownloadSize: 250000,
			MihomoPath:   defaultMihomoPath(),
			TestURL:      "https://www.google.com/generate_204",
			MaxFailCount: 10,
		},
		Scheduler: SchedulerConfig{
			Interval: 4 * time.Hour,
		},
		Extractor: ExtractorConfig{
			V2rayseURLs: []string{
				"https://test.v2rayse.com/live-node",
				"https://test.v2rayse.com/free-node",
			},
			GitHubURLs: []string{
				"https://cdn.jsdmirror.com/gh/arshiacomplus/v2rayExtractor/mix/sub.html",
			},
		},
	}

	// 从文件加载配置
	loadFromConfigFile(newConfig)
	applyEnvOverrides(newConfig)

	// 更新全局配置
	globalConfig = newConfig

	// 输出配置更新日志
	println("配置文件已更新，新配置已生效")
}

func loadFromConfigFile(cfg *Config) {
	if _, err := os.Stat(configPath); err != nil {
		return
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	var fileConfig Config
	if err := yaml.Unmarshal(data, &fileConfig); err != nil {
		return
	}

	mergeConfig(cfg, &fileConfig)
}

func mergeConfig(target, source *Config) {
	if source.Server.Addr != "" {
		target.Server.Addr = source.Server.Addr
	}

	if source.Database.Path != "" {
		target.Database.Path = source.Database.Path
	}

	if source.Verifier.Timeout != 0 {
		target.Verifier.Timeout = source.Verifier.Timeout
	}
	if source.Verifier.ChunkSize != 0 {
		target.Verifier.ChunkSize = source.Verifier.ChunkSize
	}
	target.Verifier.TestSpeed = source.Verifier.TestSpeed
	if source.Verifier.DownloadSize != 0 {
		target.Verifier.DownloadSize = source.Verifier.DownloadSize
	}
	if source.Verifier.MihomoPath != "" {
		target.Verifier.MihomoPath = source.Verifier.MihomoPath
	}
	if source.Verifier.TestURL != "" {
		target.Verifier.TestURL = source.Verifier.TestURL
	}
	if source.Verifier.MaxFailCount != 0 {
		target.Verifier.MaxFailCount = source.Verifier.MaxFailCount
	}

	if source.Scheduler.Interval != 0 {
		target.Scheduler.Interval = source.Scheduler.Interval
	}

	if len(source.Extractor.V2rayseURLs) > 0 {
		target.Extractor.V2rayseURLs = source.Extractor.V2rayseURLs
	}
	if len(source.Extractor.GitHubURLs) > 0 {
		target.Extractor.GitHubURLs = source.Extractor.GitHubURLs
	}
	if source.Extractor.V2rayse.Email != "" {
		target.Extractor.V2rayse.Email = source.Extractor.V2rayse.Email
	}
	if source.Extractor.V2rayse.Password != "" {
		target.Extractor.V2rayse.Password = source.Extractor.V2rayse.Password
	}
}

func applyEnvOverrides(cfg *Config) {
	if value := os.Getenv("SERVER_ADDR"); value != "" {
		cfg.Server.Addr = value
	}
	if value := os.Getenv("DATABASE_PATH"); value != "" {
		cfg.Database.Path = value
	}
	if value := os.Getenv("MIHOMO_PATH"); value != "" {
		cfg.Verifier.MihomoPath = value
	}
	if value := os.Getenv("TEST_URL"); value != "" {
		cfg.Verifier.TestURL = value
	}
}

func defaultMihomoPath() string {
	candidates := mihomoPathCandidates()
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func mihomoPathCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			"./bin/mihomo.exe",
			defaultWindowsMihomoPath,
			filepath.Clean(".\\mihomo.exe"),
		}
	case "linux":
		return []string{
			"./bin/mihomo",
			defaultLinuxMihomoPath,
			"/usr/local/bin/mihomo",
			"/usr/bin/mihomo",
		}
	case "darwin":
		return []string{
			"./bin/mihomo",
			defaultDarwinMihomoPath,
			"/usr/local/bin/mihomo",
			"/opt/homebrew/bin/mihomo",
		}
	default:
		return []string{"./bin/mihomo"}
	}
}

func IsWindows() bool {
	return runtime.GOOS == "windows"
}

func IsLinux() bool {
	return runtime.GOOS == "linux"
}

func IsDarwin() bool {
	return runtime.GOOS == "darwin"
}

func GetOS() string {
	return runtime.GOOS
}
