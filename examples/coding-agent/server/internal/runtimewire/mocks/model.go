package mocks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
)

// Model is a deterministic mock model implementation.
type Model struct{}

func NewModel() *Model {
	return &Model{}
}

func (m *Model) Generate(ctx context.Context, request agentreact.ModelRequest) (agent.Message, error) {
	latestUser := latestUserMessage(request.Messages)
	if strings.Contains(strings.ToLower(latestUser), "[e2e-coding-success]") {
		return e2eCodingSuccessResponse(request.Messages), nil
	}
	if strings.Contains(strings.ToLower(latestUser), "[e2e-tool-error]") {
		return e2eToolErrorResponse(request.Messages), nil
	}
	if request.Resolution == nil && strings.Contains(strings.ToLower(latestUser), "[suspend]") {
		return agent.Message{
			Role: agent.RoleAssistant,
			Requirement: &agent.PendingRequirement{
				ID:     "req-approval",
				Kind:   agent.RequirementKindApproval,
				Origin: agent.RequirementOriginModel,
				Prompt: "approve deterministic continuation",
			},
		}, nil
	}
	if strings.Contains(strings.ToLower(latestUser), "[e2e-bash-policy-denied]") {
		return agent.Message{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{
					ID:   "call-bash-denied-1",
					Name: "bash",
					Arguments: map[string]any{
						"command": "ls; pwd",
					},
				},
			},
		}, nil
	}
	if strings.Contains(strings.ToLower(latestUser), "[loop]") {
		return agent.Message{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{
					ID:   "call-loop-1",
					Name: "mock_lookup",
					Arguments: map[string]any{
						"query": "loop",
					},
				},
			},
		}, nil
	}
	if strings.Contains(strings.ToLower(latestUser), "[sleep]") {
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return agent.Message{}, ctx.Err()
		case <-timer.C:
		}
	}

	return agent.Message{
		Role:    agent.RoleAssistant,
		Content: deterministicContent(request),
	}, nil
}

func deterministicContent(request agentreact.ModelRequest) string {
	latestUser := latestUserMessage(request.Messages)
	return fmt.Sprintf(
		"mock_response messages=%d tools=%d latest_user=%q",
		len(request.Messages),
		len(request.Tools),
		latestUser,
	)
}

func latestUserMessage(messages []agent.Message) string {
	latestUser := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == agent.RoleUser {
			latestUser = messages[i].Content
			break
		}
	}
	return latestUser
}

func e2eCodingSuccessResponse(messages []agent.Message) agent.Message {
	switch {
	case !hasToolResult(messages, "write"):
		return agent.Message{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{
					ID:   "call-write-1",
					Name: "write",
					Arguments: map[string]any{
						"path":    "notes.txt",
						"content": "hello toolset\n",
					},
				},
			},
		}
	case !hasToolResult(messages, "read"):
		return agent.Message{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{
					ID:   "call-read-1",
					Name: "read",
					Arguments: map[string]any{
						"path": "notes.txt",
					},
				},
			},
		}
	case !hasToolResult(messages, "edit"):
		return agent.Message{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{
					ID:   "call-edit-1",
					Name: "edit",
					Arguments: map[string]any{
						"path": "notes.txt",
						"old":  "hello toolset",
						"new":  "hello real tools",
					},
				},
			},
		}
	case !hasToolResult(messages, "bash"):
		return agent.Message{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{
					ID:   "call-bash-1",
					Name: "bash",
					Arguments: map[string]any{
						"command": "cat notes.txt",
					},
				},
			},
		}
	default:
		return agent.Message{
			Role:    agent.RoleAssistant,
			Content: "coding flow success",
		}
	}
}

func e2eToolErrorResponse(messages []agent.Message) agent.Message {
	if !hasToolResult(messages, "read") {
		return agent.Message{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				{
					ID:   "call-read-error-1",
					Name: "read",
					Arguments: map[string]any{
						"path": "../outside.txt",
					},
				},
			},
		}
	}

	return agent.Message{
		Role:    agent.RoleAssistant,
		Content: "tool error path complete",
	}
}

func hasToolResult(messages []agent.Message, toolName string) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != agent.RoleTool {
			continue
		}
		if messages[i].Name == toolName {
			return true
		}
	}
	return false
}
