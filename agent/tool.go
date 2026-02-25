package agent

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
	CallID        string            `json:"call_id"`
	Name          string            `json:"name"`
	Content       string            `json:"content"`
	IsError       bool              `json:"is_error,omitempty"`
	FailureReason ToolFailureReason `json:"failure_reason,omitempty"`
}

// CloneToolDefinition returns a deep copy of a tool definition.
func CloneToolDefinition(in ToolDefinition) ToolDefinition {
	out := in
	if in.InputSchema != nil {
		out.InputSchema = make(map[string]any, len(in.InputSchema))
		for key, value := range in.InputSchema {
			out.InputSchema[key] = cloneJSONLikeValue(value)
		}
	}
	return out
}

// CloneToolDefinitions returns deep copies of all tool definitions.
func CloneToolDefinitions(in []ToolDefinition) []ToolDefinition {
	out := make([]ToolDefinition, len(in))
	for i := range in {
		out[i] = CloneToolDefinition(in[i])
	}
	return out
}

// ToolFailureReason is a stable machine-readable classifier for tool errors.
type ToolFailureReason string

const (
	ToolFailureReasonUnknownTool      ToolFailureReason = "unknown_tool"
	ToolFailureReasonInvalidArguments ToolFailureReason = "invalid_arguments"
	ToolFailureReasonExecutorError    ToolFailureReason = "executor_error"
)

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
		for key, value := range in.Arguments {
			out.Arguments[key] = cloneJSONLikeValue(value)
		}
	}
	return out
}

func cloneJSONLikeValue(in any) any {
	switch typed := in.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = cloneJSONLikeValue(value)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = cloneJSONLikeValue(typed[i])
		}
		return out
	default:
		return in
	}
}
