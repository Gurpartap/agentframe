package runtimewire

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
)

func TestNewRuntimeEventLogSink_NilLogger(t *testing.T) {
	t.Parallel()

	if sink := newRuntimeEventLogSink(nil, config.LogFormatText); sink != nil {
		t.Fatalf("expected nil sink for nil logger")
	}
}

func TestRuntimeEventLogSink_DebugTextFormatLogsFullEventJSONString(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sink := newRuntimeEventLogSink(logger, config.LogFormatText)
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
	if !strings.Contains(line, "event=") {
		t.Fatalf("expected event field in log output: %s", line)
	}
}

func TestRuntimeEventLogSink_DebugJSONFormatLogsNestedObject(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sink := newRuntimeEventLogSink(logger, config.LogFormatJSON)
	if sink == nil {
		t.Fatalf("expected non-nil sink")
	}

	event := agent.Event{
		RunID: "run-000002",
		Step:  3,
		Type:  agent.EventTypeAssistantMessage,
		Message: &agent.Message{
			Role:    agent.RoleAssistant,
			Content: "nested payload",
		},
	}

	if err := sink.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	line := logBuffer.String()
	if !strings.Contains(line, "\"event\":{\"run_id\":\"run-000002\"") {
		t.Fatalf("expected nested JSON event object: %s", line)
	}
	if strings.Contains(line, "\"event\":\"{\\\"run_id\\\"") {
		t.Fatalf("expected event object, found escaped string: %s", line)
	}
}

func TestRuntimeEventLogSink_InfoSkipsEventDebugLogs(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sink := newRuntimeEventLogSink(logger, config.LogFormatText)
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
	sink := newRuntimeEventLogSink(logger, config.LogFormatText)
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
