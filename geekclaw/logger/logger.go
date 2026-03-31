// Package logger 提供结构化日志功能。
package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel 表示日志级别。
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

var (
	logLevelNames = map[LogLevel]string{
		DEBUG: "DEBUG",
		INFO:  "INFO",
		WARN:  "WARN",
		ERROR: "ERROR",
		FATAL: "FATAL",
	}

	currentLevel = INFO
	logger       *Logger
	once         sync.Once
	mu           sync.RWMutex
)

// Logger 是日志记录器。
type Logger struct {
	file *os.File
}

// LogEntry 表示一条结构化日志条目。
type LogEntry struct {
	Level     string         `json:"level"`
	Timestamp string         `json:"timestamp"`
	Component string         `json:"component,omitempty"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	Caller    string         `json:"caller,omitempty"`
}

func init() {
	once.Do(func() {
		logger = &Logger{}
	})
}

// SetLevel 设置当前日志级别。
func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level
}

// GetLevel 获取当前日志级别。
func GetLevel() LogLevel {
	mu.RLock()
	defer mu.RUnlock()
	return currentLevel
}

// EnableFileLogging 启用文件日志记录。
func EnableFileLogging(filePath string) error {
	mu.Lock()
	defer mu.Unlock()

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	if logger.file != nil {
		logger.file.Close()
	}

	logger.file = file
	log.Println("File logging enabled:", filePath)
	return nil
}

// DisableFileLogging 禁用文件日志记录。
func DisableFileLogging() {
	mu.Lock()
	defer mu.Unlock()

	if logger.file != nil {
		logger.file.Close()
		logger.file = nil
		log.Println("File logging disabled")
	}
}

// logMessage 记录一条日志消息。
func logMessage(level LogLevel, component string, message string, fields map[string]any) {
	if level < currentLevel {
		return
	}

	entry := LogEntry{
		Level:     logLevelNames[level],
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Component: component,
		Message:   message,
		Fields:    fields,
	}

	if pc, file, line, ok := runtime.Caller(2); ok {
		fn := runtime.FuncForPC(pc)
		if fn != nil {
			entry.Caller = fmt.Sprintf("%s:%d (%s)", file, line, fn.Name())
		}
	}

	if logger.file != nil {
		jsonData, err := json.Marshal(entry)
		if err == nil {
			logger.file.Write(append(jsonData, '\n'))
		}
	}

	var fieldStr string
	if len(fields) > 0 {
		fieldStr = " " + formatFields(fields)
	} else {
		fieldStr = ""
	}

	logLine := fmt.Sprintf("[%s] [%s]%s %s%s",
		entry.Timestamp,
		logLevelNames[level],
		formatComponent(component),
		message,
		fieldStr,
	)

	log.Println(logLine)

	if level == FATAL {
		os.Exit(1)
	}
}

// formatComponent 格式化组件名称。
func formatComponent(component string) string {
	if component == "" {
		return ""
	}
	return fmt.Sprintf(" %s:", component)
}

// formatFields 格式化字段映射为字符串。
func formatFields(fields map[string]any) string {
	parts := make([]string, 0, len(fields))
	for k, v := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return fmt.Sprintf("{%s}", strings.Join(parts, ", "))
}

// Debug 记录调试级别日志。
func Debug(message string) {
	logMessage(DEBUG, "", message, nil)
}

// DebugC 记录带组件名的调试级别日志。
func DebugC(component string, message string) {
	logMessage(DEBUG, component, message, nil)
}

// DebugF 记录带字段的调试级别日志。
func DebugF(message string, fields map[string]any) {
	logMessage(DEBUG, "", message, fields)
}

// DebugCF 记录带组件名和字段的调试级别日志。
func DebugCF(component string, message string, fields map[string]any) {
	logMessage(DEBUG, component, message, fields)
}

// Info 记录信息级别日志。
func Info(message string) {
	logMessage(INFO, "", message, nil)
}

// InfoC 记录带组件名的信息级别日志。
func InfoC(component string, message string) {
	logMessage(INFO, component, message, nil)
}

// InfoF 记录带字段的信息级别日志。
func InfoF(message string, fields map[string]any) {
	logMessage(INFO, "", message, fields)
}

// InfoCF 记录带组件名和字段的信息级别日志。
func InfoCF(component string, message string, fields map[string]any) {
	logMessage(INFO, component, message, fields)
}

// Warn 记录警告级别日志。
func Warn(message string) {
	logMessage(WARN, "", message, nil)
}

// WarnC 记录带组件名的警告级别日志。
func WarnC(component string, message string) {
	logMessage(WARN, component, message, nil)
}

// WarnF 记录带字段的警告级别日志。
func WarnF(message string, fields map[string]any) {
	logMessage(WARN, "", message, fields)
}

// WarnCF 记录带组件名和字段的警告级别日志。
func WarnCF(component string, message string, fields map[string]any) {
	logMessage(WARN, component, message, fields)
}

// Error 记录错误级别日志。
func Error(message string) {
	logMessage(ERROR, "", message, nil)
}

// ErrorC 记录带组件名的错误级别日志。
func ErrorC(component string, message string) {
	logMessage(ERROR, component, message, nil)
}

// ErrorF 记录带字段的错误级别日志。
func ErrorF(message string, fields map[string]any) {
	logMessage(ERROR, "", message, fields)
}

// ErrorCF 记录带组件名和字段的错误级别日志。
func ErrorCF(component string, message string, fields map[string]any) {
	logMessage(ERROR, component, message, fields)
}

// Fatal 记录致命级别日志并退出程序。
func Fatal(message string) {
	logMessage(FATAL, "", message, nil)
}

// FatalC 记录带组件名的致命级别日志并退出程序。
func FatalC(component string, message string) {
	logMessage(FATAL, component, message, nil)
}

// FatalF 记录带字段的致命级别日志并退出程序。
func FatalF(message string, fields map[string]any) {
	logMessage(FATAL, "", message, fields)
}

// FatalCF 记录带组件名和字段的致命级别日志并退出程序。
func FatalCF(component string, message string, fields map[string]any) {
	logMessage(FATAL, component, message, fields)
}
