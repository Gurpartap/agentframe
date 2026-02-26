package runtimewire_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/runtimewire"
)

func TestProviderModeSmoke(t *testing.T) {
	t.Parallel()

	if os.Getenv("CODING_AGENT_PROVIDER_SMOKE") != "1" {
		t.Skip("set CODING_AGENT_PROVIDER_SMOKE=1 to run provider smoke test")
	}

	apiKey := strings.TrimSpace(os.Getenv("CODING_AGENT_PROVIDER_API_KEY"))
	if apiKey == "" {
		t.Skip("set CODING_AGENT_PROVIDER_API_KEY to run provider smoke test")
	}

	cfg := config.Default()
	cfg.ModelMode = config.ModelModeProvider
	cfg.ProviderAPIKey = apiKey
	if model := strings.TrimSpace(os.Getenv("CODING_AGENT_PROVIDER_MODEL")); model != "" {
		cfg.ProviderModel = model
	}
	if baseURL := strings.TrimSpace(os.Getenv("CODING_AGENT_PROVIDER_BASE_URL")); baseURL != "" {
		cfg.ProviderBaseURL = baseURL
	}
	if timeout := strings.TrimSpace(os.Getenv("CODING_AGENT_PROVIDER_TIMEOUT")); timeout != "" {
		parsed, err := time.ParseDuration(timeout)
		if err != nil {
			t.Fatalf("parse CODING_AGENT_PROVIDER_TIMEOUT: %v", err)
		}
		cfg.ProviderTimeout = parsed
	}

	runtime, err := runtimewire.New(cfg)
	if err != nil {
		t.Fatalf("new runtime in provider mode: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ProviderTimeout)
	defer cancel()

	result, runErr := runtime.Runner.Run(ctx, agent.RunInput{
		UserPrompt: "Reply with the single word pong.",
		MaxSteps:   2,
	})
	if runErr != nil {
		t.Fatalf("provider run error: %v", runErr)
	}
	if result.State.Status != agent.RunStatusCompleted {
		t.Fatalf("provider run status mismatch: got=%s want=%s", result.State.Status, agent.RunStatusCompleted)
	}
	if strings.TrimSpace(result.State.Output) == "" {
		t.Fatalf("provider run returned empty output")
	}
}
