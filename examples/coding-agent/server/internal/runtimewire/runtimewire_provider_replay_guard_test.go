package runtimewire_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/runtimewire"
)

func TestProviderReplayTranscriptSequencingGuard(t *testing.T) {
	t.Parallel()

	providerServer, requestCount := newSequencingGuardProviderServer(t)
	defer providerServer.Close()

	cfg := config.Default()
	cfg.ModelMode = config.ModelModeProvider
	cfg.ProviderAPIKey = "test-provider-key"
	cfg.ProviderModel = "gpt-4.1-mini"
	cfg.ProviderBaseURL = providerServer.URL
	cfg.ProviderTimeout = 2 * time.Second
	cfg.ToolMode = config.ToolModeReal
	cfg.WorkspaceRoot = t.TempDir()
	cfg.BashTimeout = 2 * time.Second

	rt, err := runtimewire.New(cfg)
	if err != nil {
		t.Fatalf("new runtime in provider mode: %v", err)
	}

	started, runErr := rt.Runner.Run(context.Background(), agent.RunInput{
		UserPrompt: "[provider-replay-sequencing-guard]",
		MaxSteps:   6,
		Tools:      rt.ToolDefinitions,
	})
	if runErr != nil {
		t.Fatalf("start run error: %v", runErr)
	}
	if started.State.Status != agent.RunStatusSuspended {
		t.Fatalf("start run status mismatch: got=%s want=%s", started.State.Status, agent.RunStatusSuspended)
	}
	if started.State.PendingRequirement == nil {
		t.Fatalf("expected pending requirement after suspended start")
	}
	if started.State.PendingRequirement.Origin != agent.RequirementOriginTool {
		t.Fatalf(
			"pending requirement origin mismatch: got=%q want=%q",
			started.State.PendingRequirement.Origin,
			agent.RequirementOriginTool,
		)
	}
	if started.State.PendingRequirement.ToolCallID != "call-bash-denied-1" {
		t.Fatalf(
			"pending requirement tool_call_id mismatch: got=%q want=%q",
			started.State.PendingRequirement.ToolCallID,
			"call-bash-denied-1",
		)
	}

	continued, continueErr := rt.Runner.Continue(context.Background(), started.State.ID, 6, rt.ToolDefinitions, &agent.Resolution{
		RequirementID: started.State.PendingRequirement.ID,
		Kind:          started.State.PendingRequirement.Kind,
		Outcome:       agent.ResolutionOutcomeApproved,
	})
	if continueErr != nil {
		t.Fatalf("continue run error: %v", continueErr)
	}
	if continued.State.Status != agent.RunStatusCompleted {
		t.Fatalf("continue run status mismatch: got=%s want=%s", continued.State.Status, agent.RunStatusCompleted)
	}
	if continued.State.Output != "provider replay sequencing accepted" {
		t.Fatalf("continue run output mismatch: got=%q", continued.State.Output)
	}
	if got := requestCount.Load(); got != 2 {
		t.Fatalf("provider request count mismatch: got=%d want=%d", got, 2)
	}
}

func newSequencingGuardProviderServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		var request providerChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}

		switch requestCount.Add(1) {
		case 1:
			writeProviderResponse(w, providerChatResponse{
				Choices: []providerChoice{
					{
						Message: providerMessage{
							Role: "assistant",
							ToolCalls: []providerToolCall{
								{
									ID:   "call-bash-denied-1",
									Type: "function",
									Function: providerToolFunction{
										Name:      "bash",
										Arguments: `{"command":"ls; pwd"}`,
									},
								},
							},
						},
					},
				},
			})
		case 2:
			if err := validateReplaySequencingRequest(request.Messages); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeProviderResponse(w, providerChatResponse{
				Choices: []providerChoice{
					{
						Message: providerMessage{
							Role:    "assistant",
							Content: "provider replay sequencing accepted",
						},
					},
				},
			})
		default:
			http.Error(w, "unexpected provider request count", http.StatusBadRequest)
		}
	}))

	return server, &requestCount
}

func validateReplaySequencingRequest(messages []providerMessage) error {
	assistantToolCallIndex := -1
	toolMessageIndexes := make([]int, 0, 1)

	for i := range messages {
		message := messages[i]
		if message.Role == "assistant" {
			for _, call := range message.ToolCalls {
				if call.ID == "call-bash-denied-1" {
					assistantToolCallIndex = i
				}
			}
			continue
		}
		if message.Role != "tool" || message.ToolCallID != "call-bash-denied-1" {
			continue
		}
		if strings.Contains(message.Content, "suspended: denied by policy") {
			return errorsf("provider request includes stale suspended tool observation")
		}
		toolMessageIndexes = append(toolMessageIndexes, i)
		if !strings.Contains(message.Content, `bash_ok command="ls; pwd"`) {
			return errorsf("provider request replay tool observation is not the approved execution result")
		}
	}

	if assistantToolCallIndex < 0 {
		return errorsf("provider request missing assistant tool call for replayed tool observation")
	}
	if len(toolMessageIndexes) != 1 {
		return errorsf("provider request expected exactly one replayed tool observation, got=%d", len(toolMessageIndexes))
	}
	if assistantToolCallIndex > toolMessageIndexes[0] {
		return errorsf("provider request has tool observation before assistant tool call")
	}
	return nil
}

func writeProviderResponse(w http.ResponseWriter, payload providerChatResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func errorsf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

type providerChatRequest struct {
	Messages []providerMessage `json:"messages"`
}

type providerChatResponse struct {
	Choices []providerChoice `json:"choices"`
}

type providerChoice struct {
	Message providerMessage `json:"message"`
}

type providerMessage struct {
	Role       string             `json:"role"`
	Content    string             `json:"content,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	ToolCalls  []providerToolCall `json:"tool_calls,omitempty"`
}

type providerToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function providerToolFunction `json:"function"`
}

type providerToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}
