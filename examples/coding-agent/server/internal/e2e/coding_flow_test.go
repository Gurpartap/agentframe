package e2e_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/runtimewire"
)

func TestCodingFlowSuccess(t *testing.T) {
	t.Parallel()

	runtime, workspaceRoot := newE2ERuntime(t)

	result, runErr := runtime.Runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "[e2e-coding-success]",
		MaxSteps:   10,
		Tools:      runtime.ToolDefinitions,
	})
	if runErr != nil {
		t.Fatalf("coding flow run error: %v", runErr)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("coding flow status mismatch: got=%s want=%s", result.State.Status, agent.RunStatusCompleted)
	}
	if !strings.Contains(result.State.Output, "coding flow success") {
		t.Fatalf("coding flow output mismatch: got=%q", result.State.Output)
	}

	content, err := os.ReadFile(filepath.Join(workspaceRoot, "notes.txt"))
	if err != nil {
		t.Fatalf("read generated notes.txt: %v", err)
	}
	if !strings.Contains(string(content), "hello real tools") {
		t.Fatalf("unexpected notes.txt content: %q", string(content))
	}
}

func TestToolErrorPath(t *testing.T) {
	t.Parallel()

	runtime, _ := newE2ERuntime(t)

	result, runErr := runtime.Runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "[e2e-tool-error]",
		MaxSteps:   4,
		Tools:      runtime.ToolDefinitions,
	})
	if runErr != nil {
		t.Fatalf("tool error path run error: %v", runErr)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("tool error path status mismatch: got=%s want=%s", result.State.Status, agent.RunStatusCompleted)
	}
	if !strings.Contains(result.State.Output, "tool error path complete") {
		t.Fatalf("tool error output mismatch: got=%q", result.State.Output)
	}

	toolMessage, ok := latestToolMessage(result.State.Messages)
	if !ok {
		t.Fatalf("expected tool message in transcript")
	}
	if toolMessage.Name != "read" {
		t.Fatalf("tool message name mismatch: got=%q want=%q", toolMessage.Name, "read")
	}
	if !strings.Contains(toolMessage.Content, "path escapes workspace root") {
		t.Fatalf("tool error message mismatch: got=%q", toolMessage.Content)
	}
}

func TestCancellationPath(t *testing.T) {
	t.Parallel()

	runtime, _ := newE2ERuntime(t)

	started, runErr := runtime.Runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "[loop] cancellation e2e",
		MaxSteps:   1,
		Tools:      runtime.ToolDefinitions,
	})
	if !errors.Is(runErr, agent.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", runErr)
	}
	if started.State.Status != agent.RunStatusMaxStepsExceeded {
		t.Fatalf("start status mismatch: got=%s want=%s", started.State.Status, agent.RunStatusMaxStepsExceeded)
	}

	cancelled, cancelErr := runtime.Runner.Cancel(context.Background(), started.State.ID)
	if cancelErr != nil {
		t.Fatalf("cancel run: %v", cancelErr)
	}
	if cancelled.State.Status != agent.RunStatusCancelled {
		t.Fatalf("cancel status mismatch: got=%s want=%s", cancelled.State.Status, agent.RunStatusCancelled)
	}
}

func newE2ERuntime(t *testing.T) (*runtimewire.Runtime, string) {
	t.Helper()

	workspaceRoot := t.TempDir()
	cfg := config.Default()
	cfg.ModelMode = config.ModelModeMock
	cfg.ToolMode = config.ToolModeReal
	cfg.WorkspaceRoot = workspaceRoot
	cfg.BashTimeout = 2 * time.Second

	runtime, err := runtimewire.New(cfg)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	return runtime, workspaceRoot
}

func latestToolMessage(messages []agent.Message) (agent.Message, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == agent.RoleTool {
			return messages[i], true
		}
	}
	return agent.Message{}, false
}
