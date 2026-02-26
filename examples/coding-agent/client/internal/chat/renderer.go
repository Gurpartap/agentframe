package chat

import (
	"io"
	"strings"
	"sync"
)

const (
	clearLineControl = "\r\033[2K"
	defaultPrompt    = "chat> "
)

type Renderer struct {
	out         io.Writer
	prompt      string
	mu          sync.Mutex
	promptShown bool
}

func NewRenderer(out io.Writer, prompt string) *Renderer {
	if out == nil {
		out = io.Discard
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = defaultPrompt
	}
	return &Renderer{
		out:    out,
		prompt: prompt,
	}
}

func (r *Renderer) ShowPrompt() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := io.WriteString(r.out, r.prompt); err != nil {
		return err
	}
	r.promptShown = true
	return nil
}

func (r *Renderer) HidePrompt() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.promptShown = false
}

func (r *Renderer) PrintLine(line string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	trimmed := strings.TrimRight(line, "\n")

	if r.promptShown {
		if _, err := io.WriteString(r.out, clearLineControl); err != nil {
			return err
		}
		if trimmed != "" {
			if _, err := io.WriteString(r.out, trimmed); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(r.out, "\n"); err != nil {
			return err
		}
		_, err := io.WriteString(r.out, r.prompt)
		return err
	}

	if trimmed != "" {
		if _, err := io.WriteString(r.out, trimmed); err != nil {
			return err
		}
	}
	_, err := io.WriteString(r.out, "\n")
	return err
}
