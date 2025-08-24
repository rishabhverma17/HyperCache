package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LogLevel represents the severity of a log entry
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ContextKey for correlation ID
type contextKey string

const CorrelationIDKey contextKey = "correlation_id"

// LogEntry represents a structured log entry for JSON serialization
type LogEntry struct {
	Timestamp     time.Time              `json:"@timestamp"`
	Level         string                 `json:"level"`
	Message       string                 `json:"message"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	NodeID        string                 `json:"node_id,omitempty"`
	Component     string                 `json:"component,omitempty"`
	Action        string                 `json:"action,omitempty"`
	Duration      *int64                 `json:"duration_ms,omitempty"`
	Error         string                 `json:"error,omitempty"`
	Fields        map[string]interface{} `json:"fields,omitempty"`
	File          string                 `json:"file,omitempty"`
	Line          int                    `json:"line,omitempty"`
	Function      string                 `json:"function,omitempty"`
}

// Logger represents the structured logger
type Logger struct {
	level   LogLevel
	nodeID  string
	writers []io.Writer
	mu      sync.RWMutex
	logChan chan LogEntry
	done    chan struct{}
	wg      sync.WaitGroup
}

// Config for logger initialization
type Config struct {
	Level         LogLevel
	NodeID        string
	LogFile       string
	EnableConsole bool
	EnableFile    bool
	BufferSize    int
}

// NewLogger creates a new structured logger instance
func NewLogger(config Config) *Logger {
	logger := &Logger{
		level:   config.Level,
		nodeID:  config.NodeID,
		writers: make([]io.Writer, 0),
		logChan: make(chan LogEntry, config.BufferSize),
		done:    make(chan struct{}),
	}

	// Add console writer if enabled
	if config.EnableConsole {
		logger.writers = append(logger.writers, os.Stdout)
	}

	// Add file writer if enabled
	if config.EnableFile && config.LogFile != "" {
		if file, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			logger.writers = append(logger.writers, file)
		} else {
			fmt.Printf("Failed to open log file %s: %v\n", config.LogFile, err)
		}
	}

	// Start log processor goroutine
	logger.wg.Add(1)
	go logger.processLogs()

	return logger
}

// processLogs handles asynchronous log writing
func (l *Logger) processLogs() {
	defer l.wg.Done()

	for {
		select {
		case entry := <-l.logChan:
			l.writeEntry(entry)
		case <-l.done:
			// Flush remaining entries
			for {
				select {
				case entry := <-l.logChan:
					l.writeEntry(entry)
				default:
					return
				}
			}
		}
	}
}

// writeEntry writes a log entry to all configured writers
func (l *Logger) writeEntry(entry LogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Printf("Failed to marshal log entry: %v\n", err)
		return
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, writer := range l.writers {
		writer.Write(data)
		writer.Write([]byte("\n"))
	}
}

// WithCorrelationID adds a correlation ID to the context
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// NewCorrelationID generates a new correlation ID
func NewCorrelationID() string {
	return uuid.New().String()
}

// GetCorrelationID retrieves the correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return id
	}
	return ""
}

// log is the internal logging method
func (l *Logger) log(ctx context.Context, level LogLevel, component, action, message string, fields map[string]interface{}, err error, duration *time.Duration) {
	if level < l.level {
		return
	}

	// Get caller information
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		file = "unknown"
		line = 0
	}

	// Get function name
	pc, _, _, ok := runtime.Caller(3)
	funcName := "unknown"
	if ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			funcName = fn.Name()
		}
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level.String(),
		Message:   message,
		NodeID:    l.nodeID,
		Component: component,
		Action:    action,
		Fields:    fields,
		File:      file,
		Line:      line,
		Function:  funcName,
	}

	// Add correlation ID if available
	if correlationID := GetCorrelationID(ctx); correlationID != "" {
		entry.CorrelationID = correlationID
	}

	// Add error if provided
	if err != nil {
		entry.Error = err.Error()
	}

	// Add duration if provided
	if duration != nil {
		durationMs := duration.Nanoseconds() / int64(time.Millisecond)
		entry.Duration = &durationMs
	}

	// Send to log channel (non-blocking)
	select {
	case l.logChan <- entry:
	default:
		// Log channel is full, write directly (fallback)
		l.writeEntry(entry)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(ctx context.Context, component, action, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ctx, DEBUG, component, action, message, f, nil, nil)
}

// Info logs an info message
func (l *Logger) Info(ctx context.Context, component, action, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ctx, INFO, component, action, message, f, nil, nil)
}

// Warn logs a warning message
func (l *Logger) Warn(ctx context.Context, component, action, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ctx, WARN, component, action, message, f, nil, nil)
}

// Error logs an error message
func (l *Logger) Error(ctx context.Context, component, action, message string, err error, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ctx, ERROR, component, action, message, f, err, nil)
}

// Fatal logs a fatal message
func (l *Logger) Fatal(ctx context.Context, component, action, message string, err error, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ctx, FATAL, component, action, message, f, err, nil)
}

// WithDuration logs with timing information
func (l *Logger) WithDuration(ctx context.Context, level LogLevel, component, action, message string, duration time.Duration, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ctx, level, component, action, message, f, nil, &duration)
}

// StartTimer returns a function that logs duration when called
func (l *Logger) StartTimer(ctx context.Context, component, action, message string) func() {
	start := time.Now()
	return func() {
		duration := time.Since(start)
		l.WithDuration(ctx, INFO, component, action, message, duration)
	}
}

// Close gracefully closes the logger
func (l *Logger) Close() {
	close(l.done)
	l.wg.Wait()

	// Close file writers
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, writer := range l.writers {
		if closer, ok := writer.(io.Closer); ok && writer != os.Stdout && writer != os.Stderr {
			closer.Close()
		}
	}
}

// AddWriter adds a new writer to the logger
func (l *Logger) AddWriter(writer io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writers = append(l.writers, writer)
}

// Global logger instance
var globalLogger *Logger
var loggerMutex sync.RWMutex

// SetGlobalLogger sets the global logger instance
func SetGlobalLogger(logger *Logger) {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()
	globalLogger = logger
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() *Logger {
	loggerMutex.RLock()
	defer loggerMutex.RUnlock()
	return globalLogger
}

// Convenience functions that use the global logger
func Debug(ctx context.Context, component, action, message string, fields ...map[string]interface{}) {
	if logger := GetGlobalLogger(); logger != nil {
		logger.Debug(ctx, component, action, message, fields...)
	}
}

func Info(ctx context.Context, component, action, message string, fields ...map[string]interface{}) {
	if logger := GetGlobalLogger(); logger != nil {
		logger.Info(ctx, component, action, message, fields...)
	}
}

func Warn(ctx context.Context, component, action, message string, fields ...map[string]interface{}) {
	if logger := GetGlobalLogger(); logger != nil {
		logger.Warn(ctx, component, action, message, fields...)
	}
}

func Error(ctx context.Context, component, action, message string, err error, fields ...map[string]interface{}) {
	if logger := GetGlobalLogger(); logger != nil {
		logger.Error(ctx, component, action, message, err, fields...)
	}
}

func Fatal(ctx context.Context, component, action, message string, err error, fields ...map[string]interface{}) {
	if logger := GetGlobalLogger(); logger != nil {
		logger.Fatal(ctx, component, action, message, err, fields...)
	}
}

func StartTimer(ctx context.Context, component, action, message string) func() {
	if logger := GetGlobalLogger(); logger != nil {
		return logger.StartTimer(ctx, component, action, message)
	}
	return func() {}
}
