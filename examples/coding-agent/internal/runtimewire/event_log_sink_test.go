package runtimewire

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
)

func TestNewRuntimeEventLogSink_NilLogger(t *testing.T) {
	t.Parallel()

	if sink := newRuntimeEventLogSink(nil); sink != nil {
		t.Fatalf("expected nil sink for nil logger")
	}
}

func TestRuntimeEventLogSink_DebugLogsFullEvent(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sink := newRuntimeEventLogSink(logger)
	if sink == nil {
		t.Fatalf("expected non-nil sink")
	}

	event := agent.Event{
		RunID: "run-000001",
		Step:  2,
		Type:  agent.EventTypeAssistantMessage,
		Message: &agent.Message{
			Role:    agent.RoleAssistant,
			Content: "full payload content",
		},
	}

	if err := sink.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	line := logBuffer.String()
	if !strings.Contains(line, "run event") {
		t.Fatalf("missing log message: %s", line)
	}
	if !strings.Contains(line, "full payload content") {
		t.Fatalf("expected full event payload in debug logs: %s", line)
	}
}

func TestRuntimeEventLogSink_InfoSkipsEventDebugLogs(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sink := newRuntimeEventLogSink(logger)
	if sink == nil {
		t.Fatalf("expected non-nil sink")
	}

	event := agent.Event{
		RunID: "run-000001",
		Step:  1,
		Type:  agent.EventTypeRunStarted,
	}
	if err := sink.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	if logBuffer.Len() != 0 {
		t.Fatalf("expected no debug output at info level, got: %s", logBuffer.String())
	}
}

func TestRuntimeEventLogSink_ContextErrors(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sink := newRuntimeEventLogSink(logger)
	event := agent.Event{
		RunID: "run-000001",
		Step:  1,
		Type:  agent.EventTypeRunStarted,
	}

	if err := sink.Publish(nil, event); !errors.Is(err, agent.ErrContextNil) {
		t.Fatalf("expected ErrContextNil, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sink.Publish(ctx, event); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
