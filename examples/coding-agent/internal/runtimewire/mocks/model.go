package mocks

import (
	"context"
	"fmt"
	"strings"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
)

// Model is a deterministic mock model implementation.
type Model struct{}

func NewModel() *Model {
	return &Model{}
}

func (m *Model) Generate(_ context.Context, request agentreact.ModelRequest) (agent.Message, error) {
	latestUser := latestUserMessage(request.Messages)
	if request.Resolution == nil && strings.Contains(strings.ToLower(latestUser), "[suspend]") {
		return agent.Message{
			Role: agent.RoleAssistant,
			Requirement: &agent.PendingRequirement{
				ID:     "req-approval",
				Kind:   agent.RequirementKindApproval,
				Prompt: "approve deterministic continuation",
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
