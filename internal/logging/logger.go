package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Level string

const (
	LevelDebug Level = "DEBUG"
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

type Logger struct {
	mu     sync.Mutex
	file   *os.File
	output io.Writer
	level  Level
}

func normalizeLevel(raw string) Level {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "DEBUG":
		return LevelDebug
	case "WARN":
		return LevelWarn
	case "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

func NewLogger(path string, level string) (*Logger, error) {
	return NewLoggerWithOutput(path, level, os.Stdout)
}

func NewLoggerWithOutput(path string, level string, output io.Writer) (*Logger, error) {
	if path == "" {
		path = filepath.Join(os.Getenv("LOCALAPPDATA"), "RcloneExplorerStatus", "logs", "service.log")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	if output == nil {
		output = io.Discard
	}
	logger := &Logger{file: file, output: output, level: normalizeLevel(level)}
	logger.Info("logger initialized", "path", path)
	return logger, nil
}

func (l *Logger) enabled(level Level) bool {
	if l == nil {
		return false
	}
	priorities := map[Level]int{LevelDebug: 10, LevelInfo: 20, LevelWarn: 30, LevelError: 40}
	return priorities[level] >= priorities[l.level]
}

func (l *Logger) Log(level Level, message string, keyValues ...string) {
	if l == nil || !l.enabled(level) {
		return
	}
	parts := make([]string, 0, len(keyValues)/2+1)
	parts = append(parts, message)
	for i := 0; i+1 < len(keyValues); i += 2 {
		key := keyValues[i]
		value := keyValues[i+1]
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}
	line := fmt.Sprintf("%s %s %s",
		time.Now().UTC().Format(time.RFC3339),
		string(level),
		strings.Join(parts, " "),
	)

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := fmt.Fprintln(l.output, line); err != nil {
		fmt.Fprintf(os.Stderr, "write live log output: %v\n", err)
	}
	if _, err := fmt.Fprintln(l.file, line); err != nil {
		fmt.Fprintf(os.Stderr, "write application log file: %v\n", err)
		return
	}
	if err := l.file.Sync(); err != nil {
		fmt.Fprintf(os.Stderr, "flush application log file: %v\n", err)
	}
}

func (l *Logger) Debug(message string, keyValues ...string) { l.Log(LevelDebug, message, keyValues...) }
func (l *Logger) Info(message string, keyValues ...string)  { l.Log(LevelInfo, message, keyValues...) }
func (l *Logger) Warn(message string, keyValues ...string)  { l.Log(LevelWarn, message, keyValues...) }
func (l *Logger) Error(message string, keyValues ...string) { l.Log(LevelError, message, keyValues...) }

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

func DefaultLogPath() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "RcloneExplorerStatus", "logs", "service.log")
}

func BuildStructuredFields(fields map[string]string) []string {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		pairs = append(pairs, key, fields[key])
	}
	return pairs
}
