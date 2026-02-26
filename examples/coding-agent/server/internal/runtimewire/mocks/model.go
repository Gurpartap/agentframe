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
	latestUser := latestUserPromptMessage(request.Messages)
	latestUserLower := strings.ToLower(latestUser)
	if strings.Contains(latestUserLower, "[e2e-coding-success]") {
		return e2eCodingSuccessResponse(request.Messages), nil
	}
	if strings.Contains(latestUserLower, "[e2e-tool-error]") {
		return e2eToolErrorResponse(request.Messages), nil
	}
	if request.Resolution == nil && strings.Contains(latestUserLower, "[suspend]") {
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
	if strings.Contains(latestUserLower, "[e2e-bash-policy-denied-next]") {
		return bashPolicyDeniedOnceResponse(request.Messages, "call-bash-denied-2"), nil
	}
	if strings.Contains(latestUserLower, "[e2e-bash-policy-two-stage]") {
		return bashPolicyTwoStageResponse(request.Messages), nil
	}
	if strings.Contains(latestUserLower, "[e2e-bash-policy-denied]") {
		return bashPolicyDeniedOnceResponse(request.Messages, "call-bash-denied-1"), nil
	}
	if strings.Contains(latestUserLower, "[loop]") {
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
	if strings.Contains(latestUserLower, "[sleep]") {
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

func bashPolicyDeniedOnceResponse(messages []agent.Message, callID string) agent.Message {
	if !hasToolResultByCallID(messages, callID) {
		return agent.Message{
			Role: agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{
				bashPolicyDeniedToolCall(callID),
			},
		}
	}

	return agent.Message{
		Role:    agent.RoleAssistant,
		Content: fmt.Sprintf("bash policy replay complete call_id=%s", callID),
	}
}

func bashPolicyTwoStageResponse(messages []agent.Message) agent.Message {
	switch {
	case !hasToolResultByCallID(messages, "call-bash-denied-1"):
		return agent.Message{
			Role:      agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{bashPolicyDeniedToolCall("call-bash-denied-1")},
		}
	case !hasToolResultByCallID(messages, "call-bash-denied-2"):
		return agent.Message{
			Role:      agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{bashPolicyDeniedToolCall("call-bash-denied-2")},
		}
	default:
		return agent.Message{
			Role:    agent.RoleAssistant,
			Content: "bash policy two stage complete",
		}
	}
}

func bashPolicyDeniedToolCall(callID string) agent.ToolCall {
	return agent.ToolCall{
		ID:   callID,
		Name: "bash",
		Arguments: map[string]any{
			"command": "ls; pwd",
		},
	}
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

func latestUserPromptMessage(messages []agent.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != agent.RoleUser {
			continue
		}
		if strings.HasPrefix(messages[i].Content, "[resolution]") {
			continue
		}
		return messages[i].Content
	}
	return latestUserMessage(messages)
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

func hasToolResultByCallID(messages []agent.Message, callID string) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != agent.RoleTool {
			continue
		}
		if messages[i].ToolCallID == callID {
			return true
		}
	}
	return false
}
