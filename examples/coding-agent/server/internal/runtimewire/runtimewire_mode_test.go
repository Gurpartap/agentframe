package runtimewire_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/runtimewire"
)

func TestRuntimeDefaultUsesMockModel(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.ModelMode = config.ModelModeMock
	cfg.ToolMode = config.ToolModeMock

	runtime, err := runtimewire.New(cfg)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, runErr := runtime.Runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "deterministic response check",
		MaxSteps:   2,
		Tools:      runtime.ToolDefinitions,
	})
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("status mismatch: got=%s want=%s", result.State.Status, agent.RunStatusCompleted)
	}
	if !strings.Contains(result.State.Output, "mock_response") {
		t.Fatalf("expected mock output marker, got=%q", result.State.Output)
	}
}

func TestRuntimeProviderModeRequiresProviderConfig(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.ModelMode = config.ModelModeProvider
	cfg.ProviderAPIKey = ""

	if _, err := runtimewire.New(cfg); err == nil {
		t.Fatalf("expected provider config validation error")
	}
}
