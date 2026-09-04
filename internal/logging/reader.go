package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLogReadBytes = int64(2 << 20)
	defaultLogEntries   = 500
	maxLogValueLength   = 500
)

var ErrInvalidLogLevel = errors.New("invalid log level")

type ReaderConfig struct {
	FilePath     string
	MaxReadBytes int64
	MaxEntries   int
}

type Reader struct {
	filePath     string
	maxReadBytes int64
	maxEntries   int
}

type LogQuery struct {
	Limit  int
	Level  string
	Search string
}

type LogSnapshot struct {
	Entries      []LogEntry
	Scanned      int
	Matched      int
	InfoCount    int
	WarnCount    int
	ErrorCount   int
	DebugCount   int
	FileSize     int64
	FileSizeText string
	LastModified string
	Partial      bool
}

type LogEntry struct {
	Time       string
	Level      string
	LevelClass string
	Message    string
	Attributes []LogAttribute
}

type LogAttribute struct {
	Name  string
	Value string
}

func NewReader(config ReaderConfig) (*Reader, error) {
	if strings.TrimSpace(config.FilePath) == "" {
		return nil, errors.New("logging: log reader file path is required")
	}
	filePath, err := filepath.Abs(config.FilePath)
	if err != nil {
		return nil, fmt.Errorf("resolve log reader path: %w", err)
	}
	if config.MaxReadBytes == 0 {
		config.MaxReadBytes = defaultLogReadBytes
	}
	if config.MaxEntries == 0 {
		config.MaxEntries = defaultLogEntries
	}
	if config.MaxReadBytes < 1024 || config.MaxEntries < 1 {
		return nil, errors.New("logging: log reader limits must be positive")
	}
	return &Reader{filePath: filePath, maxReadBytes: config.MaxReadBytes, maxEntries: config.MaxEntries}, nil
}

func (reader *Reader) Read(query LogQuery) (LogSnapshot, error) {
	level := strings.ToUpper(strings.TrimSpace(query.Level))
	if level == "ALL" {
		level = ""
	}
	if level != "" && level != "DEBUG" && level != "INFO" && level != "WARN" && level != "ERROR" {
		return LogSnapshot{}, ErrInvalidLogLevel
	}
	limit := query.Limit
	if limit < 1 {
		limit = 100
	}
	if limit > reader.maxEntries {
		limit = reader.maxEntries
	}
	search := strings.ToLower(strings.TrimSpace(query.Search))

	file, err := os.Open(reader.filePath)
	if err != nil {
		return LogSnapshot{}, fmt.Errorf("open application log: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return LogSnapshot{}, fmt.Errorf("inspect application log: %w", err)
	}
	if !info.Mode().IsRegular() {
		return LogSnapshot{}, errors.New("application log is not a regular file")
	}

	start := max(info.Size()-reader.maxReadBytes, 0)
	if _, err := file.Seek(start, 0); err != nil {
		return LogSnapshot{}, fmt.Errorf("seek application log: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(file, info.Size()-start))
	if err != nil {
		return LogSnapshot{}, fmt.Errorf("read application log: %w", err)
	}
	if start > 0 {
		if boundary := bytes.IndexByte(data, '\n'); boundary >= 0 {
			data = data[boundary+1:]
		} else {
			data = nil
		}
	}

	snapshot := LogSnapshot{
		FileSize:     info.Size(),
		FileSizeText: formatFileSize(info.Size()),
		LastModified: info.ModTime().UTC().Format("2006-01-02 15:04:05 UTC"),
		Partial:      start > 0,
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		entry, ok := parseLogEntry(line)
		if !ok {
			continue
		}
		snapshot.Scanned++
		switch entry.Level {
		case "DEBUG":
			snapshot.DebugCount++
		case "INFO":
			snapshot.InfoCount++
		case "WARN":
			snapshot.WarnCount++
		case "ERROR":
			snapshot.ErrorCount++
		}
		if level != "" && entry.Level != level {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(string(line)), search) {
			continue
		}
		snapshot.Matched++
		snapshot.Entries = append(snapshot.Entries, entry)
		if len(snapshot.Entries) > limit {
			copy(snapshot.Entries, snapshot.Entries[1:])
			snapshot.Entries = snapshot.Entries[:limit]
		}
	}
	for left, right := 0, len(snapshot.Entries)-1; left < right; left, right = left+1, right-1 {
		snapshot.Entries[left], snapshot.Entries[right] = snapshot.Entries[right], snapshot.Entries[left]
	}
	return snapshot, nil
}

func parseLogEntry(line []byte) (LogEntry, bool) {
	var record map[string]any
	if err := json.Unmarshal(line, &record); err != nil {
		return LogEntry{}, false
	}
	level, _ := record["level"].(string)
	level = strings.ToUpper(level)
	if level == "" {
		level = "INFO"
	}
	message, _ := record["msg"].(string)
	entry := LogEntry{
		Time:       formatLogTime(record["time"]),
		Level:      level,
		LevelClass: levelClass(level),
		Message:    truncateLogValue(message),
	}
	keys := make([]string, 0, len(record))
	for key := range record {
		if key != "time" && key != "level" && key != "msg" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry.Attributes = append(entry.Attributes, LogAttribute{Name: key, Value: safeLogValue(key, record[key])})
	}
	return entry, true
}

func formatLogTime(value any) string {
	raw, _ := value.(string)
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return truncateLogValue(raw)
	}
	return parsed.UTC().Format("2006-01-02 15:04:05.000 UTC")
}

func levelClass(level string) string {
	switch level {
	case "ERROR":
		return "danger"
	case "WARN":
		return "warning"
	case "DEBUG":
		return "secondary"
	default:
		return "primary"
	}
}

func safeLogValue(key string, value any) string {
	normalized := strings.ToLower(key)
	for _, sensitive := range []string{"password", "passwd", "secret", "token", "authorization", "cookie", "session"} {
		if strings.Contains(normalized, sensitive) {
			return "[REDACTED]"
		}
	}
	if text, ok := value.(string); ok {
		return truncateLogValue(text)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "[UNAVAILABLE]"
	}
	return truncateLogValue(string(encoded))
}

func truncateLogValue(value string) string {
	runes := []rune(value)
	if len(runes) <= maxLogValueLength {
		return value
	}
	return string(runes[:maxLogValueLength]) + "…"
}

func formatFileSize(size int64) string {
	if size < 1024 {
		return strconv.FormatInt(size, 10) + " B"
	}
	if size < 1<<20 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
}
