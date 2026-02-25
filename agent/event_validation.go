package agent

import "fmt"

// ValidateEvent checks event payload invariants before publish boundaries.
func ValidateEvent(event Event) error {
	if event.Type == "" {
		return fmt.Errorf("%w: field=type reason=empty", ErrEventInvalid)
	}
	if event.RunID == "" {
		return fmt.Errorf("%w: field=run_id reason=empty type=%s", ErrEventInvalid, event.Type)
	}
	if event.Step < 0 {
		return fmt.Errorf(
			"%w: field=step reason=negative value=%d type=%s run_id=%q",
			ErrEventInvalid,
			event.Step,
			event.Type,
			event.RunID,
		)
	}

	switch event.Type {
	case EventTypeCommandApplied:
		if event.CommandKind == "" {
			return fmt.Errorf(
				"%w: field=command_kind reason=empty type=%s run_id=%q step=%d",
				ErrEventInvalid,
				event.Type,
				event.RunID,
				event.Step,
			)
		}
	case EventTypeAssistantMessage:
		if event.Message == nil {
			return fmt.Errorf(
				"%w: field=message reason=nil type=%s run_id=%q step=%d",
				ErrEventInvalid,
				event.Type,
				event.RunID,
				event.Step,
			)
		}
	case EventTypeToolResult:
		if event.ToolResult == nil {
			return fmt.Errorf(
				"%w: field=tool_result reason=nil type=%s run_id=%q step=%d",
				ErrEventInvalid,
				event.Type,
				event.RunID,
				event.Step,
			)
		}
		if event.ToolResult.CallID == "" {
			return fmt.Errorf(
				"%w: field=tool_result.call_id reason=empty type=%s run_id=%q step=%d",
				ErrEventInvalid,
				event.Type,
				event.RunID,
				event.Step,
			)
		}
		if event.ToolResult.Name == "" {
			return fmt.Errorf(
				"%w: field=tool_result.name reason=empty type=%s run_id=%q step=%d",
				ErrEventInvalid,
				event.Type,
				event.RunID,
				event.Step,
			)
		}
	}

	return nil
}
