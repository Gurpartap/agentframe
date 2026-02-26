package clientchat

import (
	"bytes"
	"context"
	"reflect"
	"strconv"
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
		Continue: func(_ context.Context, maxSteps *int) error {
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
