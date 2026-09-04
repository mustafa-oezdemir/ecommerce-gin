package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPServerAppliesLimits(t *testing.T) {
	config := testHTTPConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := newHTTPServer(":8080", http.NotFoundHandler(), config, logger)

	if server.ReadHeaderTimeout != config.ReadHeaderTimeout || server.ReadTimeout != config.ReadTimeout || server.WriteTimeout != config.WriteTimeout || server.IdleTimeout != config.IdleTimeout {
		t.Fatal("HTTP timeouts were not applied")
	}
	if server.MaxHeaderBytes != config.MaxHeaderBytes {
		t.Fatalf("expected MaxHeaderBytes %d, got %d", config.MaxHeaderBytes, server.MaxHeaderBytes)
	}
}

func TestGroupStartsAndShutsDownWhenContextIsCancelled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	group, err := NewGroup("127.0.0.1:0", http.NotFoundHandler(), "127.0.0.1:0", http.NotFoundHandler(), testHTTPConfig(), logger)
	if err != nil {
		t.Fatalf("create server group: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := group.Run(ctx); err != nil {
		t.Fatalf("run server group: %v", err)
	}
}

func TestNewGroupRejectsUnsafeLimits(t *testing.T) {
	config := testHTTPConfig()
	config.MaxHeaderBytes = 1024
	_, err := NewGroup(":8080", http.NotFoundHandler(), ":9091", http.NotFoundHandler(), config, nil)
	if err == nil {
		t.Fatal("expected unsafe MaxHeaderBytes to fail")
	}
}

func testHTTPConfig() HTTPConfig {
	return HTTPConfig{
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
		ShutdownTimeout:   time.Second,
		MaxHeaderBytes:    32 * 1024,
	}
}
