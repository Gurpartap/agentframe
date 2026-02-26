package clientchat

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPromptResolutionUsesDefaultsAndTypedFields(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(
		"\n" + // requirement_id -> default
			"\n" + // kind -> default
			"approved\n" +
			"manual note\n",
	)
	var out bytes.Buffer
	renderer := NewRenderer(&out, "chat> ")

	resolution, err := PromptResolution(
		context.Background(),
		bufio.NewReader(input),
		renderer,
		ResolutionPromptDefaults{
			RequirementID: "req-approval",
			Kind:          "approval",
			Prompt:        "approve deterministic continuation",
		},
	)
	if err != nil {
		t.Fatalf("prompt resolution: %v", err)
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
	if resolution.Value != "manual note" {
		t.Fatalf("value mismatch: got=%q want=%q", resolution.Value, "manual note")
	}
}

func TestPromptResolutionRejectsInvalidKindAndOutcome(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(
		"req-approval\n" +
			"bad_kind\n" +
			"approval\n" +
			"bad_outcome\n" +
			"approved\n" +
			"\n",
	)
	var out bytes.Buffer
	renderer := NewRenderer(&out, "chat> ")

	resolution, err := PromptResolution(context.Background(), bufio.NewReader(input), renderer, ResolutionPromptDefaults{})
	if err != nil {
		t.Fatalf("prompt resolution: %v", err)
	}
	if resolution.Kind != "approval" {
		t.Fatalf("kind mismatch: got=%q want=%q", resolution.Kind, "approval")
	}
	if resolution.Outcome != "approved" {
		t.Fatalf("outcome mismatch: got=%q want=%q", resolution.Outcome, "approved")
	}

	rendered := out.String()
	if !strings.Contains(rendered, "unsupported requirement kind") {
		t.Fatalf("expected invalid kind guidance in output: %q", rendered)
	}
	if !strings.Contains(rendered, "unsupported resolution outcome") {
		t.Fatalf("expected invalid outcome guidance in output: %q", rendered)
	}
}

func TestResolutionValidators(t *testing.T) {
	t.Parallel()

	if err := ValidateRequirementKind("approval"); err != nil {
		t.Fatalf("validate requirement kind: %v", err)
	}
	if err := ValidateRequirementKind("unknown"); err == nil {
		t.Fatalf("expected invalid requirement kind")
	}

	if err := ValidateResolutionOutcome("approved"); err != nil {
		t.Fatalf("validate outcome: %v", err)
	}
	if err := ValidateResolutionOutcome("unknown"); err == nil {
		t.Fatalf("expected invalid resolution outcome")
	}
}
