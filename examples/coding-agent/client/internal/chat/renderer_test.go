package chat

import (
	"bytes"
	"strings"
	"testing"
)

func TestRendererPromptRedrawOnBackgroundLine(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	renderer := NewRenderer(&out, "chat> ")

	if err := renderer.ShowPrompt(); err != nil {
		t.Fatalf("show prompt: %v", err)
	}
	if err := renderer.PrintLine("event arrived"); err != nil {
		t.Fatalf("print line: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "chat> ") {
		t.Fatalf("expected prompt in output: %q", got)
	}
	if !strings.Contains(got, clearLineControl+"event arrived\nchat> ") {
		t.Fatalf("expected clear/redraw sequence, got: %q", got)
	}
}

func TestRendererPrintLineWithoutPrompt(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	renderer := NewRenderer(&out, "chat> ")

	if err := renderer.PrintLine("plain output"); err != nil {
		t.Fatalf("print line: %v", err)
	}

	if got := out.String(); got != "plain output\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}
