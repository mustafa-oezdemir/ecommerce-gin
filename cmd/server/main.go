package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/config"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/logging"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	appserver "github.com/mustafa-oezdemir/ecommerce-gin/internal/server"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/uploads"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "application stopped: %v\n", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	cfg := config.Load()
	logRuntime, err := logging.New(logging.Config{
		Environment:   cfg.AppEnv,
		Level:         cfg.LogLevel,
		ConsoleFormat: cfg.LogConsoleFormat,
		FilePath:      cfg.LogFile,
		MaxSizeMB:     cfg.LogMaxSizeMB,
		MaxBackups:    cfg.LogMaxBackups,
		MaxAgeDays:    cfg.LogMaxAgeDays,
		Compress:      cfg.LogCompress,
		AddSource:     cfg.LogAddSource,
	})
	if err != nil {
		return fmt.Errorf("initialize logging: %w", err)
	}
	defer func() {
		if err := logRuntime.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close log file: %w", err))
		}
	}()
	slog.SetDefault(logRuntime.Logger)
	defer func() {
		if runErr != nil {
			slog.Error("application stopped with errors", "error", runErr)
		} else {
			slog.Info("application stopped cleanly")
		}
	}()
	gin.SetMode(cfg.GinMode)
	gin.DisableConsoleColor()
	gin.DefaultWriter = logging.NewWriter(slog.Default(), slog.LevelDebug, "gin")
	gin.DefaultErrorWriter = logging.NewWriter(slog.Default(), slog.LevelError, "gin")
	gin.DebugPrintRouteFunc = func(method, path, handler string, handlerCount int) {
		slog.Debug("route registered", "method", method, "route", path, "handler", handler, "handler_count", handlerCount)
	}
	slog.Info("logging initialized", "file", logRuntime.FilePath(), "minimum_level", cfg.LogLevel, "console_format", cfg.LogConsoleFormat)
	database, err := db.Open(cfg)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer func() {
		if err := db.Close(database); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close database: %w", err))
		}
	}()

	appMetrics := metrics.New(prometheus.DefaultRegisterer)
	if sqlDB, err := db.SQL(database); err == nil {
		prometheus.MustRegister(collectors.NewDBStatsCollector(sqlDB, "ecommerce"))
	}
	appMetrics.HealthLive.Set(1)
	metrics.SetDefault(appMetrics)
	malwareScanner, err := uploads.NewClamAVScanner(cfg.ClamAVAddress, cfg.ClamAVScanTimeout)
	if err != nil {
		return fmt.Errorf("configure malware scanner: %w", err)
	}
	imageStore, err := uploads.NewImageStore(uploads.ImageConfig{
		Directory: cfg.ProductImageDirectory,
		MaxBytes:  cfg.ProductImageMaxBytes,
		MaxWidth:  cfg.ProductImageMaxWidth,
		MaxHeight: cfg.ProductImageMaxHeight,
		MaxPixels: cfg.ProductImageMaxPixels,
		Scanner:   malwareScanner,
	})
	if err != nil {
		return fmt.Errorf("configure product image storage: %w", err)
	}
	logReader, err := logging.NewReader(logging.ReaderConfig{FilePath: logRuntime.FilePath()})
	if err != nil {
		return fmt.Errorf("configure application log reader: %w", err)
	}
	applicationHandler, err := appserver.NewRouter(appserver.RouterConfig{
		Environment:    cfg.AppEnv,
		TrustedProxies: cfg.TrustedProxies,
		SessionSecret:  cfg.SessionSecret,
		SessionSecure:  cfg.SessionSecure,
		CSRFKey:        cfg.CSRFKey,
		SecurityKey:    cfg.SecurityEncryptionKey,
		Database:       database,
		Metrics:        appMetrics,
		Logger:         slog.Default(),
		ImageStore:     imageStore,
		LogReader:      logReader,
	})
	if err != nil {
		return fmt.Errorf("build application router: %w", err)
	}
	serverGroup, err := appserver.NewGroup(
		":"+cfg.AppPort,
		applicationHandler,
		":"+cfg.MetricsPort,
		promhttp.Handler(),
		appserver.HTTPConfig{
			ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
			ReadTimeout:       cfg.HTTPReadTimeout,
			WriteTimeout:      cfg.HTTPWriteTimeout,
			IdleTimeout:       cfg.HTTPIdleTimeout,
			ShutdownTimeout:   cfg.HTTPShutdownTimeout,
			MaxHeaderBytes:    cfg.HTTPMaxHeaderBytes,
		},
		slog.Default(),
	)
	if err != nil {
		return fmt.Errorf("build HTTP servers: %w", err)
	}

	serverContext, stopServers := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopServers()
	if err := serverGroup.Run(serverContext); err != nil {
		runErr = errors.Join(runErr, err)
	}
	return runErr
}
