package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxLogFileSize = 10 * 1024 * 1024 // 10MB
	logDir         = "logs"
	logFileName    = "app.log"
)

type Logger struct {
	consoleLogger *log.Logger
	fileLogger    *log.Logger
	file          *os.File
	mu            sync.Mutex
}

var (
	instance *Logger
	once     sync.Once
)

func GetLogger() *Logger {
	once.Do(func() {
		instance = newLogger()
	})
	return instance
}

func newLogger() *Logger {
	// 创建日志目录
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("创建日志目录失败: %v", err)
	}

	// 打开日志文件
	logPath := filepath.Join(logDir, logFileName)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("打开日志文件失败: %v", err)
		return &Logger{
			consoleLogger: log.New(os.Stdout, "", 0),
			fileLogger:    nil,
			file:          nil,
		}
	}

	// 检查并轮转日志文件
	if err := rotateLogFileIfNeeded(logPath); err != nil {
		log.Printf("检查日志轮转失败: %v", err)
	}

	// 重新打开文件（可能在轮转后被删除）
	file, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("重新打开日志文件失败: %v", err)
		return &Logger{
			consoleLogger: log.New(os.Stdout, "", 0),
			fileLogger:    nil,
			file:          nil,
		}
	}

	return &Logger{
		consoleLogger: log.New(os.Stdout, "", 0),
		fileLogger:    log.New(file, "", 0),
		file:          file,
	}
}

// 用于存储多输出器
type multiWriter struct {
	writers []io.Writer
}

func (m *multiWriter) Write(p []byte) (n int, err error) {
	for _, w := range m.writers {
		n, err = w.Write(p)
		if err != nil {
			return n, err
		}
		if n != len(p) {
			return n, io.ErrShortWrite
		}
	}
	return len(p), nil
}

var multiWriterInstance *multiWriter

func (l *Logger) checkAndRotate() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	// 获取文件信息
	info, err := l.file.Stat()
	if err != nil {
		log.Printf("获取日志文件信息失败: %v", err)
		return
	}

	// 检查文件大小
	if info.Size() >= maxLogFileSize {
		// 关闭当前文件
		l.file.Close()

		// 轮转日志
		logPath := filepath.Join(logDir, logFileName)
		if err := rotateLogFile(logPath); err != nil {
			log.Printf("轮转日志失败: %v", err)
		}

		// 重新打开文件
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("重新打开日志文件失败: %v", err)
			l.file = nil
			l.fileLogger = nil
			return
		}

		l.file = file
		l.fileLogger = log.New(file, "", log.LstdFlags)
	}
}

func rotateLogFileIfNeeded(logPath string) error {
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.Size() >= maxLogFileSize {
		return rotateLogFile(logPath)
	}

	return nil
}

func rotateLogFile(logPath string) error {
	// 备份旧日志
	backupPath := filepath.Join(logDir, fmt.Sprintf("app_%s.log", time.Now().Format("20060102_150405")))

	// 如果备份文件已存在，直接删除
	if _, err := os.Stat(backupPath); err == nil {
		os.Remove(backupPath)
	}

	// 重命名当前日志文件
	if err := os.Rename(logPath, backupPath); err != nil {
		// 如果重命名失败，尝试直接删除原文件
		if err := os.Remove(logPath); err != nil {
			return fmt.Errorf("删除旧日志文件失败: %w", err)
		}
	}

	// 清理旧的备份文件，只保留最新的几个
	cleanupOldLogs()

	return nil
}

func cleanupOldLogs() {
	// 获取所有日志备份文件
	pattern := filepath.Join(logDir, "app_*.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		log.Printf("获取日志备份文件失败: %v", err)
		return
	}

	// 如果备份文件太多，删除最旧的
	if len(files) > 5 {
		// 按修改时间排序
		type fileInfo struct {
			path string
			info os.FileInfo
		}

		fileInfos := make([]fileInfo, 0, len(files))
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			fileInfos = append(fileInfos, fileInfo{path: f, info: info})
		}

		// 按修改时间排序（最旧的在前）
		for i := 0; i < len(fileInfos)-1; i++ {
			for j := i + 1; j < len(fileInfos); j++ {
				if fileInfos[i].info.ModTime().After(fileInfos[j].info.ModTime()) {
					fileInfos[i], fileInfos[j] = fileInfos[j], fileInfos[i]
				}
			}
		}

		// 删除最旧的文件，只保留5个
		for i := 0; i < len(fileInfos)-5; i++ {
			if err := os.Remove(fileInfos[i].path); err != nil {
				log.Printf("删除旧日志文件失败 %s: %v", fileInfos[i].path, err)
			} else {
				log.Printf("删除旧日志文件: %s", fileInfos[i].path)
			}
		}
	}
}

func (l *Logger) output(level string, format string, v ...interface{}) {
	l.checkAndRotate()

	msg := fmt.Sprintf(format, v...)
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	logLine := fmt.Sprintf("%s %s %s", timestamp, level, msg)

	// 输出到控制台
	l.consoleLogger.Println(logLine)

	// 输出到文件
	l.mu.Lock()
	if l.fileLogger != nil {
		l.fileLogger.Println(logLine)
	}
	l.mu.Unlock()
}

func (l *Logger) Printf(format string, v ...interface{}) {
	l.output("", format, v...)
}

func (l *Logger) Println(v ...interface{}) {
	l.output("", fmt.Sprint(v...))
}

func (l *Logger) Fatal(v ...interface{}) {
	l.output("FATAL", fmt.Sprint(v...))
	os.Exit(1)
}

func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.output("FATAL", format, v...)
	os.Exit(1)
}

func (l *Logger) Fatalln(v ...interface{}) {
	l.output("FATAL", fmt.Sprint(v...))
	os.Exit(1)
}

func (l *Logger) Panic(v ...interface{}) {
	s := fmt.Sprint(v...)
	l.output("PANIC", s)
	panic(s)
}

func (l *Logger) Panicf(format string, v ...interface{}) {
	s := fmt.Sprintf(format, v...)
	l.output("PANIC", s)
	panic(s)
}

func (l *Logger) Panicln(v ...interface{}) {
	s := fmt.Sprint(v...)
	l.output("PANIC", s)
	panic(s)
}

// 包级别的便捷函数
func Printf(format string, v ...interface{}) {
	GetLogger().Printf(format, v...)
}

func Println(v ...interface{}) {
	GetLogger().Println(v...)
}

func Fatal(v ...interface{}) {
	GetLogger().Fatal(v...)
}

func Fatalf(format string, v ...interface{}) {
	GetLogger().Fatalf(format, v...)
}

func Fatalln(v ...interface{}) {
	GetLogger().Fatalln(v...)
}

func Panic(v ...interface{}) {
	GetLogger().Panic(v...)
}

func Panicf(format string, v ...interface{}) {
	GetLogger().Panicf(format, v...)
}

func Panicln(v ...interface{}) {
	GetLogger().Panicln(v...)
}
