package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
)

func TestNewServerLogger_JSONFormat(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := newServerLogger(&out, slog.LevelInfo, config.LogFormatJSON)
	logger.Info("json log test", slog.String("key", "value"))

	line := out.String()
	if !strings.Contains(line, "\"msg\":\"json log test\"") {
		t.Fatalf("expected json message field, got: %s", line)
	}
	if !strings.Contains(line, "\"key\":\"value\"") {
		t.Fatalf("expected json key field, got: %s", line)
	}
}

func TestNewServerLogger_TextFormat(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := newServerLogger(&out, slog.LevelInfo, config.LogFormatText)
	logger.Info("text log test", slog.String("key", "value"))

	line := out.String()
	if !strings.Contains(line, "text log test") {
		t.Fatalf("expected text message, got: %s", line)
	}
	if !strings.Contains(line, "key=") {
		t.Fatalf("expected text key field, got: %s", line)
	}
}
