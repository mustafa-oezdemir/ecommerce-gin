package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeWritesStructuredRotatingFileAndFilteredConsole(t *testing.T) {
	var console bytes.Buffer
	logPath := filepath.Join(t.TempDir(), "nested", "application.log")
	runtime, err := New(Config{
		Environment:   "test",
		Level:         "info",
		ConsoleFormat: "text",
		FilePath:      logPath,
		MaxSizeMB:     1,
		MaxBackups:    2,
		MaxAgeDays:    7,
		Compress:      true,
		Console:       &console,
	})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	runtime.Logger.Debug("hidden debug message")
	runtime.Logger.Info("application started", "port", 8080)
	runtime.Logger.Warn("request rejected", "status", 429)
	runtime.Logger.Error("database unavailable", "retryable", true)
	if err := runtime.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	contents := string(data)
	if strings.Contains(contents, "hidden debug message") {
		t.Fatal("debug record was not filtered")
	}
	for _, expected := range []string{`"level":"INFO"`, `"level":"WARN"`, `"level":"ERROR"`, `"service":"ecommerce"`, `"environment":"test"`} {
		if !strings.Contains(contents, expected) {
			t.Fatalf("log file does not contain %s: %s", expected, contents)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(contents), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line is not valid JSON: %q: %v", line, err)
		}
	}
	if !strings.Contains(console.String(), "level=WARN") || !strings.Contains(console.String(), "level=ERROR") {
		t.Fatalf("console did not receive expected levels: %s", console.String())
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("second close should be safe: %v", err)
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	_, err := New(Config{Level: "verbose", ConsoleFormat: "text", FilePath: "app.log", MaxSizeMB: 1, MaxBackups: 1, MaxAgeDays: 1})
	if err == nil {
		t.Fatal("expected invalid log level to fail")
	}

	_, err = New(Config{Level: "info", ConsoleFormat: "text", FilePath: t.TempDir(), MaxSizeMB: 1, MaxBackups: 1, MaxAgeDays: 1})
	if err == nil {
		t.Fatal("expected an unwritable log file path to fail")
	}
}
