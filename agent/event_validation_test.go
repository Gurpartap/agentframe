package agent

import (
	"context"
	"errors"
	"testing"
)

func TestValidateEventMatrix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		event   Event
		wantErr string
	}{
		{
			name: "valid run started",
			event: Event{
				RunID: "run-1",
				Step:  0,
				Type:  EventTypeRunStarted,
			},
		},
		{
			name: "valid command applied",
			event: Event{
				RunID:       "run-1",
				Step:        2,
				Type:        EventTypeCommandApplied,
				CommandKind: CommandKindContinue,
			},
		},
		{
			name: "valid assistant message",
			event: Event{
				RunID: "run-1",
				Step:  1,
				Type:  EventTypeAssistantMessage,
				Message: &Message{
					Role:    RoleAssistant,
					Content: "hello",
				},
			},
		},
		{
			name: "valid tool result",
			event: Event{
				RunID: "run-1",
				Step:  1,
				Type:  EventTypeToolResult,
				ToolResult: &ToolResult{
					CallID: "call-1",
					Name:   "lookup",
				},
			},
		},
		{
			name: "missing type",
			event: Event{
				RunID: "run-1",
				Step:  0,
			},
			wantErr: "event is invalid: field=type reason=empty",
		},
		{
			name: "missing run id",
			event: Event{
				RunID: "",
				Step:  0,
				Type:  EventTypeRunStarted,
			},
			wantErr: "event is invalid: field=run_id reason=empty type=run_started",
		},
		{
			name: "negative step",
			event: Event{
				RunID: "run-1",
				Step:  -1,
				Type:  EventTypeRunStarted,
			},
			wantErr: "event is invalid: field=step reason=negative value=-1 type=run_started run_id=\"run-1\"",
		},
		{
			name: "unknown event type",
			event: Event{
				RunID: "run-1",
				Step:  0,
				Type:  EventType("mystery"),
			},
			wantErr: "event is invalid: field=type reason=unknown value=\"mystery\" run_id=\"run-1\" step=0",
		},
		{
			name: "command applied missing command kind",
			event: Event{
				RunID: "run-1",
				Step:  0,
				Type:  EventTypeCommandApplied,
			},
			wantErr: "event is invalid: field=command_kind reason=empty type=command_applied run_id=\"run-1\" step=0",
		},
		{
			name: "command applied forbids message",
			event: Event{
				RunID: "run-1",
				Step:  0,
				Type:  EventTypeCommandApplied,
				Message: &Message{
					Role:    RoleAssistant,
					Content: "unexpected",
				},
				CommandKind: CommandKindStart,
			},
			wantErr: "event is invalid: field=message reason=forbidden type=command_applied run_id=\"run-1\" step=0",
		},
		{
			name: "command applied forbids tool result",
			event: Event{
				RunID:       "run-1",
				Step:        0,
				Type:        EventTypeCommandApplied,
				CommandKind: CommandKindStart,
				ToolResult: &ToolResult{
					CallID: "call-1",
					Name:   "lookup",
				},
			},
			wantErr: "event is invalid: field=tool_result reason=forbidden type=command_applied run_id=\"run-1\" step=0",
		},
		{
			name: "assistant message missing payload",
			event: Event{
				RunID: "run-1",
				Step:  1,
				Type:  EventTypeAssistantMessage,
			},
			wantErr: "event is invalid: field=message reason=nil type=assistant_message run_id=\"run-1\" step=1",
		},
		{
			name: "assistant message forbids command kind",
			event: Event{
				RunID:       "run-1",
				Step:        1,
				Type:        EventTypeAssistantMessage,
				CommandKind: CommandKindContinue,
				Message: &Message{
					Role:    RoleAssistant,
					Content: "hello",
				},
			},
			wantErr: "event is invalid: field=command_kind reason=forbidden value=\"continue\" type=assistant_message run_id=\"run-1\" step=1",
		},
		{
			name: "assistant message forbids tool result",
			event: Event{
				RunID: "run-1",
				Step:  1,
				Type:  EventTypeAssistantMessage,
				Message: &Message{
					Role:    RoleAssistant,
					Content: "hello",
				},
				ToolResult: &ToolResult{
					CallID: "call-1",
					Name:   "lookup",
				},
			},
			wantErr: "event is invalid: field=tool_result reason=forbidden type=assistant_message run_id=\"run-1\" step=1",
		},
		{
			name: "tool result missing payload",
			event: Event{
				RunID: "run-1",
				Step:  1,
				Type:  EventTypeToolResult,
			},
			wantErr: "event is invalid: field=tool_result reason=nil type=tool_result run_id=\"run-1\" step=1",
		},
		{
			name: "tool result forbids command kind",
			event: Event{
				RunID:       "run-1",
				Step:        1,
				Type:        EventTypeToolResult,
				CommandKind: CommandKindFollowUp,
				ToolResult: &ToolResult{
					CallID: "call-1",
					Name:   "lookup",
				},
			},
			wantErr: "event is invalid: field=command_kind reason=forbidden value=\"follow_up\" type=tool_result run_id=\"run-1\" step=1",
		},
		{
			name: "tool result forbids message",
			event: Event{
				RunID: "run-1",
				Step:  1,
				Type:  EventTypeToolResult,
				Message: &Message{
					Role:    RoleAssistant,
					Content: "unexpected",
				},
				ToolResult: &ToolResult{
					CallID: "call-1",
					Name:   "lookup",
				},
			},
			wantErr: "event is invalid: field=message reason=forbidden type=tool_result run_id=\"run-1\" step=1",
		},
		{
			name: "tool result missing call id",
			event: Event{
				RunID: "run-1",
				Step:  1,
				Type:  EventTypeToolResult,
				ToolResult: &ToolResult{
					CallID: "",
					Name:   "lookup",
				},
			},
			wantErr: "event is invalid: field=tool_result.call_id reason=empty type=tool_result run_id=\"run-1\" step=1",
		},
		{
			name: "tool result missing name",
			event: Event{
				RunID: "run-1",
				Step:  1,
				Type:  EventTypeToolResult,
				ToolResult: &ToolResult{
					CallID: "call-1",
					Name:   "",
				},
			},
			wantErr: "event is invalid: field=tool_result.name reason=empty type=tool_result run_id=\"run-1\" step=1",
		},
		{
			name: "run started forbids command kind",
			event: Event{
				RunID:       "run-1",
				Step:        0,
				Type:        EventTypeRunStarted,
				CommandKind: CommandKindStart,
			},
			wantErr: "event is invalid: field=command_kind reason=forbidden value=\"start\" type=run_started run_id=\"run-1\" step=0",
		},
		{
			name: "run completed forbids message",
			event: Event{
				RunID: "run-1",
				Step:  2,
				Type:  EventTypeRunCompleted,
				Message: &Message{
					Role:    RoleAssistant,
					Content: "unexpected",
				},
			},
			wantErr: "event is invalid: field=message reason=forbidden type=run_completed run_id=\"run-1\" step=2",
		},
		{
			name: "run checkpoint forbids tool result",
			event: Event{
				RunID: "run-1",
				Step:  3,
				Type:  EventTypeRunCheckpoint,
				ToolResult: &ToolResult{
					CallID: "call-1",
					Name:   "lookup",
				},
			},
			wantErr: "event is invalid: field=tool_result reason=forbidden type=run_checkpoint run_id=\"run-1\" step=3",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateEvent(tc.event)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if !errors.Is(err, ErrEventInvalid) {
				t.Fatalf("expected ErrEventInvalid, got %v", err)
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("unexpected error text: got=%q want=%q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestPublishEventRejectsInvalidPayloadBeforeSink(t *testing.T) {
	t.Parallel()

	sink := &countingEventSink{}
	err := publishEvent(context.Background(), sink, Event{
		RunID: "run-1",
		Step:  0,
		Type:  EventTypeCommandApplied,
	})
	if !errors.Is(err, ErrEventInvalid) {
		t.Fatalf("expected ErrEventInvalid, got %v", err)
	}
	if sink.calls != 0 {
		t.Fatalf("sink should not be called for invalid event, got calls=%d", sink.calls)
	}
}

func TestPublishEventPublishesValidPayload(t *testing.T) {
	t.Parallel()

	sink := &countingEventSink{}
	err := publishEvent(context.Background(), sink, Event{
		RunID:       "run-1",
		Step:        0,
		Type:        EventTypeCommandApplied,
		CommandKind: CommandKindStart,
	})
	if err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if sink.calls != 1 {
		t.Fatalf("expected one sink call, got %d", sink.calls)
	}
}

type countingEventSink struct {
	calls int
}

func (s *countingEventSink) Publish(context.Context, Event) error {
	s.calls++
	return nil
}
