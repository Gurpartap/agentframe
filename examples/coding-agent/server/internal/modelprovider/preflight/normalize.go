package preflight

import (
	"fmt"
	"strings"

	"github.com/Gurpartap/agentframe/agent"
)

// NormalizeMessagesForProvider validates and normalizes transcripts for provider payloads.
func NormalizeMessagesForProvider(messages []agent.Message) ([]agent.Message, error) {
	normalized := make([]agent.Message, 0, len(messages))
	assistantToolCalls := make(map[string]struct{}, len(messages))
	toolMessageIndexByCallID := make(map[string]int, len(messages))

	for i := range messages {
		message := agent.CloneMessage(messages[i])
		switch message.Role {
		case agent.RoleAssistant:
			normalized = append(normalized, message)
			for _, call := range message.ToolCalls {
				if call.ID == "" {
					continue
				}
				assistantToolCalls[call.ID] = struct{}{}
			}
		case agent.RoleTool:
			toolCallID := strings.TrimSpace(message.ToolCallID)
			if toolCallID == "" {
				return nil, fmt.Errorf("decode messages: tool message at index %d missing tool_call_id", i)
			}
			if _, ok := assistantToolCalls[toolCallID]; !ok {
				return nil, fmt.Errorf(
					"decode messages: tool message at index %d references unknown tool_call_id %q",
					i,
					toolCallID,
				)
			}
			if existingIndex, exists := toolMessageIndexByCallID[toolCallID]; exists {
				// Keep only the latest tool observation for a call in provider payloads.
				normalized[existingIndex] = message
			} else {
				normalized = append(normalized, message)
				toolMessageIndexByCallID[toolCallID] = len(normalized) - 1
			}
		default:
			normalized = append(normalized, message)
		}
	}
	return normalized, nil
}
