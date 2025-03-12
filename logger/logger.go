package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Level 日志级别
type Level int

const (
	// DEBUG 调试级别
	DEBUG Level = iota
	// INFO 信息级别
	INFO
	// WARN 警告级别
	WARN
	// ERROR 错误级别
	ERROR
	// FATAL 致命错误级别
	FATAL
)

var levelNames = map[Level]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

// Logger 日志记录器
type Logger struct {
	level     Level
	output    io.Writer
	logger    *log.Logger
	mu        sync.Mutex
	fields    map[string]interface{}
	callDepth int
}

// Options 日志选项
type Options struct {
	Level     Level
	Output    io.Writer
	CallDepth int
	Fields    map[string]interface{}
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// DefaultLogger 获取默认日志记录器
func DefaultLogger() *Logger {
	once.Do(func() {
		defaultLogger = NewLogger(&Options{
			Level:  INFO,
			Output: os.Stdout,
		})
	})
	return defaultLogger
}

// NewLogger 创建新的日志记录器
func NewLogger(opts *Options) *Logger {
	if opts == nil {
		opts = &Options{
			Level:  INFO,
			Output: os.Stdout,
		}
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}
	if opts.Fields == nil {
		opts.Fields = make(map[string]interface{})
	}

	return &Logger{
		level:     opts.Level,
		output:    opts.Output,
		logger:    log.New(opts.Output, "", 0),
		fields:    opts.Fields,
		callDepth: opts.CallDepth,
	}
}

// WithField 添加字段
func (l *Logger) WithField(key string, value interface{}) *Logger {
	newLogger := l.clone()
	newLogger.fields[key] = value
	return newLogger
}

// WithFields 添加多个字段
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newLogger := l.clone()
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	return newLogger
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel 获取日志级别
func (l *Logger) GetLevel() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// SetOutput 设置输出
func (l *Logger) SetOutput(output io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = output
	l.logger = log.New(output, "", 0)
}

func (l *Logger) clone() *Logger {
	fields := make(map[string]interface{}, len(l.fields))
	for k, v := range l.fields {
		fields[k] = v
	}
	return &Logger{
		level:     l.level,
		output:    l.output,
		logger:    l.logger,
		fields:    fields,
		callDepth: l.callDepth,
	}
}

func (l *Logger) log(level Level, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 获取调用信息
	_, file, line, ok := runtime.Caller(l.callDepth + 2)
	if !ok {
		file = "???"
		line = 0
	}
	file = filepath.Base(file)

	// 构建日志消息
	msg := fmt.Sprint(args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	levelName := levelNames[level]

	// 构建字段字符串
	fields := ""
	if len(l.fields) > 0 {
		fields = " "
		for k, v := range l.fields {
			fields += fmt.Sprintf("%s=%v ", k, v)
		}
		fields = fields[:len(fields)-1]
	}

	// 格式化并写入日志
	logMsg := fmt.Sprintf("%s [%s] %s:%d%s %s\n",
		timestamp, levelName, file, line, fields, msg)
	l.logger.Output(0, logMsg)

	if level == FATAL {
		os.Exit(1)
	}
}

func (l *Logger) logf(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}
	l.log(level, fmt.Sprintf(format, args...))
}

// Debug 调试日志
func (l *Logger) Debug(args ...interface{}) {
	l.log(DEBUG, args...)
}

// Debugf 格式化调试日志
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.logf(DEBUG, format, args...)
}

// Info 信息日志
func (l *Logger) Info(args ...interface{}) {
	l.log(INFO, args...)
}

// Infof 格式化信息日志
func (l *Logger) Infof(format string, args ...interface{}) {
	l.logf(INFO, format, args...)
}

// Warn 警告日志
func (l *Logger) Warn(args ...interface{}) {
	l.log(WARN, args...)
}

// Warnf 格式化警告日志
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.logf(WARN, format, args...)
}

// Error 错误日志
func (l *Logger) Error(args ...interface{}) {
	l.log(ERROR, args...)
}

// Errorf 格式化错误日志
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.logf(ERROR, format, args...)
}

// Fatal 致命错误日志
func (l *Logger) Fatal(args ...interface{}) {
	l.log(FATAL, args...)
}

// Fatalf 格式化致命错误日志
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.logf(FATAL, format, args...)
}
