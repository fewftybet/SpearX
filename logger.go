package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

var logger *Logger

type Logger struct {
	file         *os.File
	errorFile    *os.File
	mu           sync.Mutex
	errorMu      sync.Mutex
	logLevel     int
	logDir       string
	errorLogPath string
}

const (
	DEBUG = iota
	INFO
	WARN
	ERROR
)

// InitLogger 初始化日志系统
// 日志存放在 <项目根>/data/ 目录下，与其他配置/缓存保持一致
func InitLogger() error {
	// 通过向上查找 go.mod 来定位项目根，再在 data 子目录建立日志
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %v", err)
	}
	_ = execPath // 仅用于兼容

	logDir := findDataDir()
	logFileName := fmt.Sprintf("spearx_%s.log", time.Now().Format("20060102"))
	logPath := filepath.Join(logDir, logFileName)
	errorLogPath := filepath.Join(logDir, "error.log")

	// 打开常规日志文件
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("创建日志文件失败: %v", err)
	}

	// 打开错误日志文件（每次启动清空，仅保留当前会话的错误）
	errorFile, err := os.OpenFile(errorLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		logFile.Close()
		return fmt.Errorf("创建错误日志文件失败: %v", err)
	}

	logger = &Logger{
		file:         logFile,
		errorFile:    errorFile,
		logLevel:     DEBUG,
		logDir:       logDir,
		errorLogPath: errorLogPath,
	}

	Info("日志系统初始化完成")
	Info("常规日志: %s", logPath)
	Info("错误日志: %s", errorLogPath)
	return nil
}

// findDataDir 定位项目根目录的 data 子目录
// 优先级 1：从可执行文件位置向上查找 go.mod
// 优先级 2：从当前工作目录向上查找 go.mod
// 兜底：使用可执行文件所在目录
func findDataDir() string {
	// 优先级 1: 从可执行文件位置向上查找
	if execPath, err := os.Executable(); err == nil {
		dir := filepath.Dir(execPath)
		for i := 0; i < 8; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				dataDir := filepath.Join(dir, "data")
				_ = os.MkdirAll(dataDir, 0755)
				return dataDir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		// 兜底：用 exe 目录
		dataDir := filepath.Join(filepath.Dir(execPath), "data")
		_ = os.MkdirAll(dataDir, 0755)
		return dataDir
	}
	// 优先级 2: 从当前工作目录向上查找
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 8; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				dataDir := filepath.Join(dir, "data")
				_ = os.MkdirAll(dataDir, 0755)
				return dataDir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	// 兜底：当前目录
	dataDir := "data"
	_ = os.MkdirAll(dataDir, 0755)
	return dataDir
}

func (l *Logger) log(level int, format string, args ...interface{}) {
	if level < l.logLevel {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	var levelStr string
	switch level {
	case DEBUG:
		levelStr = "[DEBUG]"
	case INFO:
		levelStr = "[INFO]"
	case WARN:
		levelStr = "[WARN]"
	case ERROR:
		levelStr = "[ERROR]"
	default:
		levelStr = "[UNKNOWN]"
	}

	message := fmt.Sprintf("%s %s %s\n", timestamp, levelStr, fmt.Sprintf(format, args...))

	fmt.Print(message)

	if l.file != nil {
		l.file.WriteString(message)
		l.file.Sync()
	}

	// ERROR 级别同时写入 error.log
	if level == ERROR && l.errorFile != nil {
		l.writeErrorLog(levelStr, fmt.Sprintf(format, args...))
	}
}

// writeErrorLog 写入错误日志到 error.log，包含堆栈信息
func (l *Logger) writeErrorLog(levelStr, message string) {
	l.errorMu.Lock()
	defer l.errorMu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	header := fmt.Sprintf("\n========== %s %s ==========\n", timestamp, levelStr)
	stack := string(debug.Stack())

	entry := fmt.Sprintf("%s%s\n堆栈信息:\n%s", header, message, stack)

	fmt.Fprint(os.Stderr, entry)

	if l.errorFile != nil {
		l.errorFile.WriteString(entry)
		l.errorFile.Sync()
	}
}

// GetErrorLogPath 获取错误日志文件路径
func (l *Logger) GetErrorLogPath() string {
	if l == nil {
		return ""
	}
	return l.errorLogPath
}

// LogErrorWithContext 记录带上下文的错误（用于前端调用或关键错误点）
func LogErrorWithContext(context string, err error) {
	if err == nil {
		return
	}
	if logger != nil {
		// 写入错误日志（不重复写入常规日志，避免双写）
		logger.writeErrorLog("[ERROR]", fmt.Sprintf("[%s] %v", context, err))
	} else {
		fmt.Fprintf(os.Stderr, "[ERROR] [%s] %v\n", context, err)
	}
}

func Debug(format string, args ...interface{}) {
	if logger != nil {
		logger.log(DEBUG, format, args...)
	} else {
		fmt.Printf("[DEBUG] %s\n", fmt.Sprintf(format, args...))
	}
}

func Info(format string, args ...interface{}) {
	if logger != nil {
		logger.log(INFO, format, args...)
	} else {
		fmt.Printf("[INFO] %s\n", fmt.Sprintf(format, args...))
	}
}

func Warn(format string, args ...interface{}) {
	if logger != nil {
		logger.log(WARN, format, args...)
	} else {
		fmt.Printf("[WARN] %s\n", fmt.Sprintf(format, args...))
	}
}

func Error(format string, args ...interface{}) {
	if logger != nil {
		logger.log(ERROR, format, args...)
	} else {
		fmt.Printf("[ERROR] %s\n", fmt.Sprintf(format, args...))
	}
}

func CloseLogger() {
	if logger != nil {
		if logger.file != nil {
			logger.file.Close()
		}
		if logger.errorFile != nil {
			// 关闭错误日志时写入分隔线
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			logger.errorFile.WriteString(fmt.Sprintf("\n========== 会话结束: %s ==========\n", timestamp))
			logger.errorFile.Sync()
			logger.errorFile.Close()
		}
	}
}
