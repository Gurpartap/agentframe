package agent

// EventType is emitted by the runtime and loop for observability and streaming.
type EventType string

const (
	EventTypeCommandApplied   EventType = "command_applied"
	EventTypeRunStarted       EventType = "run_started"
	EventTypeAssistantMessage EventType = "assistant_message"
	EventTypeToolResult       EventType = "tool_result"
	EventTypeRunCompleted     EventType = "run_completed"
	EventTypeRunFailed        EventType = "run_failed"
	EventTypeRunSuspended     EventType = "run_suspended"
	EventTypeRunCancelled     EventType = "run_cancelled"
	EventTypeRunCheckpoint    EventType = "run_checkpoint"
)

// Event is intentionally compact so adapters can map it to logs, metrics, or streams.
type Event struct {
	RunID       RunID       `json:"run_id"`
	Step        int         `json:"step"`
	Type        EventType   `json:"type"`
	CommandKind CommandKind `json:"command_kind,omitempty"`
	Message     *Message    `json:"message,omitempty"`
	ToolResult  *ToolResult `json:"tool_result,omitempty"`
	Description string      `json:"description,omitempty"`
}
