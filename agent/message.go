package agent

// Role identifies the author of a message in the conversation transcript.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is the shared transport object passed between runtime, model, and tools.
type Message struct {
	Role        Role                `json:"role"`
	Content     string              `json:"content,omitempty"`
	Name        string              `json:"name,omitempty"`
	ToolCallID  string              `json:"tool_call_id,omitempty"`
	ToolCalls   []ToolCall          `json:"tool_calls,omitempty"`
	Requirement *PendingRequirement `json:"requirement,omitempty"`
}

// CloneMessage returns a deep copy suitable for isolation across component boundaries.
func CloneMessage(in Message) Message {
	out := in
	if len(in.ToolCalls) > 0 {
		out.ToolCalls = make([]ToolCall, len(in.ToolCalls))
		for i := range in.ToolCalls {
			out.ToolCalls[i] = CloneToolCall(in.ToolCalls[i])
		}
	}
	if in.Requirement != nil {
		requirementCopy := *in.Requirement
		out.Requirement = &requirementCopy
	}
	return out
}

// CloneMessages returns deep copies of all messages.
func CloneMessages(in []Message) []Message {
	out := make([]Message, len(in))
	for i := range in {
		out[i] = CloneMessage(in[i])
	}
	return out
}
