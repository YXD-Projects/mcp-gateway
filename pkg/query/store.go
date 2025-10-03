package query

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var latestQuery atomic.Value

func SetLatestQuery(q string) {
	latestQuery.Store(q)
}

func GetLatestQuery() string {
	if v := latestQuery.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

type Record struct {
	Timestamp  time.Time   `json:"ts"`
	SessionID  string      `json:"session_id"`
	ClientName string      `json:"client_name,omitempty"`
	Method     string      `json:"method"`
	ToolName   string      `json:"tool_name,omitempty"`
	ServerName string      `json:"server_name,omitempty"`
	Arguments  interface{} `json:"arguments,omitempty"`
}

var (
	logFileOnce sync.Once
	logFilePath string
	logMu       sync.Mutex
)

func resolveLogPath() string {
	logFileOnce.Do(func() {
		logFilePath = os.Getenv("DOCKER_MCP_QUERY_LOG")
	})
	return logFilePath
}

func AppendRecord(rec Record) error {
	if resolveLogPath() == "" {
		return nil
	}

	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}

	buf, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal query record: %w", err)
	}

	logMu.Lock()
	defer logMu.Unlock()

	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open query log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(buf, '\n')); err != nil {
		return fmt.Errorf("write query log: %w", err)
	}
	return nil
}
