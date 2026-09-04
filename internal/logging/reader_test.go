package logging

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReaderReturnsFilteredNewestEntriesAndRedactsSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.log")
	contents := strings.Join([]string{
		`{"time":"2026-09-04T10:00:00Z","level":"INFO","msg":"started","service":"ecommerce"}`,
		`not-json`,
		`{"time":"2026-09-04T10:01:00Z","level":"WARN","msg":"slow request","route":"/checkout","duration_ms":450,"session_token":"hidden"}`,
		`{"time":"2026-09-04T10:02:00Z","level":"ERROR","msg":"database unavailable","retryable":true}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
	reader, err := NewReader(ReaderConfig{FilePath: path, MaxReadBytes: 4096, MaxEntries: 10})
	if err != nil {
		t.Fatalf("create log reader: %v", err)
	}
	snapshot, err := reader.Read(LogQuery{Limit: 2, Search: "request"})
	if err != nil {
		t.Fatalf("read logs: %v", err)
	}
	if snapshot.Scanned != 3 || snapshot.Matched != 1 || len(snapshot.Entries) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	entry := snapshot.Entries[0]
	if entry.Level != "WARN" || entry.Message != "slow request" || entry.Time != "2026-09-04 10:01:00.000 UTC" {
		t.Fatalf("unexpected entry: %#v", entry)
	}
	for _, attribute := range entry.Attributes {
		if attribute.Name == "session_token" && attribute.Value != "[REDACTED]" {
			t.Fatalf("sensitive value was not redacted: %#v", attribute)
		}
	}

	snapshot, err = reader.Read(LogQuery{Limit: 1, Level: "error"})
	if err != nil || len(snapshot.Entries) != 1 || snapshot.Entries[0].Message != "database unavailable" {
		t.Fatalf("level filter failed: snapshot=%#v err=%v", snapshot, err)
	}
}

func TestReaderLimitsWindowAndRejectsInvalidLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.log")
	prefix := strings.Repeat("x", 1500) + "\n"
	valid := `{"time":"2026-09-04T10:02:00Z","level":"INFO","msg":"latest"}` + "\n"
	if err := os.WriteFile(path, []byte(prefix+valid), 0o600); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
	reader, err := NewReader(ReaderConfig{FilePath: path, MaxReadBytes: 1024, MaxEntries: 10})
	if err != nil {
		t.Fatalf("create log reader: %v", err)
	}
	snapshot, err := reader.Read(LogQuery{})
	if err != nil || !snapshot.Partial || len(snapshot.Entries) != 1 || snapshot.Entries[0].Message != "latest" {
		t.Fatalf("bounded read failed: snapshot=%#v err=%v", snapshot, err)
	}
	if _, err := reader.Read(LogQuery{Level: "fatal"}); !errors.Is(err, ErrInvalidLogLevel) {
		t.Fatalf("expected invalid level error, got %v", err)
	}
}
