package agent

import "maps"

// ToolDefinition declares a callable capability exposed to the model.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// ToolCall is requested by the assistant message and executed by ToolExecutor.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolResult is the normalized output produced by a tool execution.
type ToolResult struct {
	CallID  string `json:"call_id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// ToolResultMessage converts a tool result to a transcript message.
func ToolResultMessage(result ToolResult) Message {
	return Message{
		Role:       RoleTool,
		Name:       result.Name,
		ToolCallID: result.CallID,
		Content:    result.Content,
	}
}

// CloneToolCall returns a deep copy of a tool call.
func CloneToolCall(in ToolCall) ToolCall {
	out := in
	if in.Arguments != nil {
		out.Arguments = make(map[string]any, len(in.Arguments))
		maps.Copy(out.Arguments, in.Arguments)
	}
	return out
}
