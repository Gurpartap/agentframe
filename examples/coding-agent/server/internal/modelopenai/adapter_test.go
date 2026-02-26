package modelopenai

import (
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
)

func TestBuildRequest_DedupesRepeatedToolObservationByCallID(t *testing.T) {
	t.Parallel()

	request, err := buildRequest(
		"gpt-4.1-mini",
		agentreact.ModelRequest{
			Messages: []agent.Message{
				{Role: agent.RoleUser, Content: `Call bash with "git status"`},
				{
					Role: agent.RoleAssistant,
					ToolCalls: []agent.ToolCall{
						{
							ID:   "call-1",
							Name: "bash",
							Arguments: map[string]any{
								"command": "git status",
							},
						},
					},
				},
				{
					Role:       agent.RoleTool,
					ToolCallID: "call-1",
					Name:       "bash",
					Content:    "suspended: denied by policy",
				},
				{Role: agent.RoleUser, Content: `[resolution] requirement_id="req-1" kind=approval outcome=approved`},
				{
					Role:       agent.RoleTool,
					ToolCallID: "call-1",
					Name:       "bash",
					Content:    `bash_ok command="git status" stdout="clean" stderr=""`,
				},
			},
			Tools: []agent.ToolDefinition{
				{
					Name:        "bash",
					Description: "run shell command",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("buildRequest returned error: %v", err)
	}

	if len(request.Messages) != 4 {
		t.Fatalf("provider messages length mismatch: got=%d want=%d", len(request.Messages), 4)
	}
	if request.Messages[1].Role != "assistant" {
		t.Fatalf("assistant role mismatch: got=%q want=%q", request.Messages[1].Role, "assistant")
	}
	if request.Messages[2].Role != "tool" {
		t.Fatalf("tool role mismatch: got=%q want=%q", request.Messages[2].Role, "tool")
	}
	if request.Messages[2].ToolCallID != "call-1" {
		t.Fatalf("tool call id mismatch: got=%q want=%q", request.Messages[2].ToolCallID, "call-1")
	}
	if request.Messages[2].Content != `bash_ok command="git status" stdout="clean" stderr=""` {
		t.Fatalf("tool content mismatch: got=%q", request.Messages[2].Content)
	}
	if request.Messages[3].Role != "user" {
		t.Fatalf("user role mismatch: got=%q want=%q", request.Messages[3].Role, "user")
	}
}

func TestBuildRequest_RejectsToolObservationWithoutAssistantToolCall(t *testing.T) {
	t.Parallel()

	_, err := buildRequest(
		"gpt-4.1-mini",
		agentreact.ModelRequest{
			Messages: []agent.Message{
				{Role: agent.RoleUser, Content: "hello"},
				{
					Role:       agent.RoleTool,
					ToolCallID: "call-unknown",
					Name:       "bash",
					Content:    "result",
				},
			},
		},
	)
	if err == nil {
		t.Fatalf("expected buildRequest to fail when tool observation has no assistant tool call")
	}
}
