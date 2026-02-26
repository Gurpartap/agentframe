package chat

import (
	"bytes"
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestREPLSlashCommandsAndFreeText(t *testing.T) {
	t.Parallel()

	input := bytes.NewBufferString(
		"/start bootstrap run\n" +
			"/status\n" +
			"/continue --max-steps 3\n" +
			"/steer shift approach\n" +
			"/followup explicit followup\n" +
			"implicit followup\n" +
			"/cancel\n" +
			"/quit\n",
	)

	var out bytes.Buffer
	renderer := NewRenderer(&out, "chat> ")

	var called []string
	repl := NewREPL(input, renderer, Handlers{
		Start: func(_ context.Context, prompt string) error {
			called = append(called, "start:"+prompt)
			return nil
		},
		Status: func(_ context.Context) error {
			called = append(called, "status")
			return nil
		},
		Continue: func(_ context.Context, maxSteps *int, _ *ResolutionInput) error {
			if maxSteps == nil {
				called = append(called, "continue:nil")
				return nil
			}
			called = append(called, "continue:"+strconv.Itoa(*maxSteps))
			return nil
		},
		Steer: func(_ context.Context, instruction string) error {
			called = append(called, "steer:"+instruction)
			return nil
		},
		FollowUp: func(_ context.Context, prompt string) error {
			called = append(called, "followup:"+prompt)
			return nil
		},
		Cancel: func(_ context.Context) error {
			called = append(called, "cancel")
			return nil
		},
	})

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("run repl: %v", err)
	}

	expected := []string{
		"start:bootstrap run",
		"status",
		"continue:3",
		"steer:shift approach",
		"followup:explicit followup",
		"followup:implicit followup",
		"cancel",
	}
	if !reflect.DeepEqual(called, expected) {
		t.Fatalf("dispatch mismatch:\n got: %#v\nwant: %#v", called, expected)
	}
}

func TestREPLInvalidCommandPrintsErrorAndContinues(t *testing.T) {
	t.Parallel()

	input := bytes.NewBufferString("/unknown something\n/quit\n")

	var out bytes.Buffer
	renderer := NewRenderer(&out, "chat> ")
	repl := NewREPL(input, renderer, Handlers{})

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("run repl: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("error: unsupported command")) {
		t.Fatalf("expected error output, got %q", out.String())
	}
}

func TestREPLContinueResolutionPromptFlow(t *testing.T) {
	t.Parallel()

	input := bytes.NewBufferString(
		"/continue\n" +
			"\n" +
			"\n" +
			"approved\n" +
			"manual confirmation\n" +
			"/quit\n",
	)

	var out bytes.Buffer
	renderer := NewRenderer(&out, "chat> ")

	calls := 0
	repl := NewREPL(input, renderer, Handlers{
		Continue: func(_ context.Context, maxSteps *int, resolution *ResolutionInput) error {
			calls++
			if calls == 1 {
				if maxSteps != nil {
					t.Fatalf("expected nil max steps in first continue call, got %v", *maxSteps)
				}
				return NewResolutionRequiredError(ResolutionPromptDefaults{
					RequirementID: "req-approval",
					Kind:          "approval",
					Prompt:        "approve deterministic continuation",
				})
			}

			if resolution == nil {
				t.Fatalf("expected prompted resolution payload on second continue call")
			}
			if resolution.RequirementID != "req-approval" {
				t.Fatalf("requirement_id mismatch: got=%q want=%q", resolution.RequirementID, "req-approval")
			}
			if resolution.Kind != "approval" {
				t.Fatalf("kind mismatch: got=%q want=%q", resolution.Kind, "approval")
			}
			if resolution.Outcome != "approved" {
				t.Fatalf("outcome mismatch: got=%q want=%q", resolution.Outcome, "approved")
			}
			if resolution.Value != "manual confirmation" {
				t.Fatalf("value mismatch: got=%q want=%q", resolution.Value, "manual confirmation")
			}
			return nil
		},
	})

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("run repl: %v", err)
	}
	if calls != 2 {
		t.Fatalf("continue call count mismatch: got=%d want=%d", calls, 2)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "run is suspended and requires a resolution payload") {
		t.Fatalf("missing resolution guidance output: %q", rendered)
	}
	if !strings.Contains(rendered, "outcome (approved|rejected|provided|completed):") {
		t.Fatalf("missing explicit outcome prompt: %q", rendered)
	}
}
