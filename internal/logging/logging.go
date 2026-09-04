package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	Environment   string
	Level         string
	ConsoleFormat string
	FilePath      string
	MaxSizeMB     int
	MaxBackups    int
	MaxAgeDays    int
	Compress      bool
	AddSource     bool
	Console       io.Writer
}

type Runtime struct {
	Logger    *slog.Logger
	filePath  string
	fileSink  *lumberjack.Logger
	closeOnce sync.Once
	closeErr  error
}

func New(config Config) (*Runtime, error) {
	level, err := parseLevel(config.Level)
	if err != nil {
		return nil, err
	}
	if config.ConsoleFormat != "text" && config.ConsoleFormat != "json" {
		return nil, fmt.Errorf("unsupported console log format %q", config.ConsoleFormat)
	}
	if strings.TrimSpace(config.FilePath) == "" {
		return nil, errors.New("log file path is required")
	}
	if config.MaxSizeMB < 1 || config.MaxBackups < 1 || config.MaxAgeDays < 1 {
		return nil, errors.New("log rotation limits must be positive")
	}

	filePath, err := filepath.Abs(config.FilePath)
	if err != nil {
		return nil, fmt.Errorf("resolve log file path: %w", err)
	}
	fileDirectory := filepath.Dir(filePath)
	if err := os.MkdirAll(fileDirectory, 0o750); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	logRoot, err := os.OpenRoot(fileDirectory)
	if err != nil {
		return nil, fmt.Errorf("open log directory: %w", err)
	}
	logFile, err := logRoot.OpenFile(filepath.Base(filePath), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		_ = logRoot.Close()
		return nil, fmt.Errorf("open log file: %w", err)
	}
	if closeErr := errors.Join(logFile.Close(), logRoot.Close()); closeErr != nil {
		return nil, fmt.Errorf("close log file probe: %w", closeErr)
	}

	fileSink := &lumberjack.Logger{
		Filename:   filePath,
		MaxSize:    config.MaxSizeMB,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAgeDays,
		Compress:   config.Compress,
		LocalTime:  false,
	}
	handlerOptions := &slog.HandlerOptions{Level: level, AddSource: config.AddSource}
	fileHandler := slog.NewJSONHandler(fileSink, handlerOptions)
	console := config.Console
	if console == nil {
		console = os.Stdout
	}
	var consoleHandler slog.Handler
	if config.ConsoleFormat == "json" {
		consoleHandler = slog.NewJSONHandler(console, handlerOptions)
	} else {
		consoleHandler = slog.NewTextHandler(console, handlerOptions)
	}

	logger := slog.New(fanoutHandler{handlers: []slog.Handler{consoleHandler, fileHandler}}).With(
		"service", "ecommerce",
		"environment", config.Environment,
	)
	return &Runtime{Logger: logger, filePath: filePath, fileSink: fileSink}, nil
}

func (runtime *Runtime) FilePath() string {
	return runtime.filePath
}

func (runtime *Runtime) Close() error {
	runtime.closeOnce.Do(func() {
		runtime.closeErr = runtime.fileSink.Close()
	})
	return runtime.closeErr
}

func NewWriter(logger *slog.Logger, level slog.Level, component string) io.Writer {
	return &levelWriter{logger: logger, level: level, component: component}
}

type levelWriter struct {
	logger    *slog.Logger
	level     slog.Level
	component string
}

func (writer *levelWriter) Write(data []byte) (int, error) {
	message := strings.TrimSpace(string(data))
	if message != "" {
		writer.logger.Log(context.Background(), writer.level, message, "component", writer.component)
	}
	return len(data), nil
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", value)
	}
}

type fanoutHandler struct {
	handlers []slog.Handler
}

func (handler fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, target := range handler.handlers {
		if target.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (handler fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	var handlerErrors []error
	for _, target := range handler.handlers {
		if target.Enabled(ctx, record.Level) {
			if err := target.Handle(ctx, record.Clone()); err != nil {
				handlerErrors = append(handlerErrors, err)
			}
		}
	}
	return errors.Join(handlerErrors...)
}

func (handler fanoutHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	targets := make([]slog.Handler, 0, len(handler.handlers))
	for _, target := range handler.handlers {
		targets = append(targets, target.WithAttrs(attributes))
	}
	return fanoutHandler{handlers: targets}
}

func (handler fanoutHandler) WithGroup(name string) slog.Handler {
	targets := make([]slog.Handler, 0, len(handler.handlers))
	for _, target := range handler.handlers {
		targets = append(targets, target.WithGroup(name))
	}
	return fanoutHandler{handlers: targets}
}
