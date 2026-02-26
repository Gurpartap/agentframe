package clientcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/clientapi"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/clientchat"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/clientevents"
)

func runChat(ctx context.Context, api *clientapi.Client, baseURL string, input io.Reader, output io.Writer) error {
	controller := newChatController(ctx, api, baseURL, output)
	defer controller.stopStream()

	renderer := controller.renderer
	if err := renderer.PrintLine("chat mode ready. slash commands: /start /status /continue /steer /followup /cancel /quit"); err != nil {
		return err
	}
	if err := renderer.PrintLine("free text sends /followup on the active run"); err != nil {
		return err
	}

	repl := clientchat.NewREPL(input, renderer, clientchat.Handlers{
		Start:    controller.start,
		Status:   controller.status,
		Continue: controller.continueRun,
		Steer:    controller.steer,
		FollowUp: controller.followUp,
		Cancel:   controller.cancel,
	})

	return repl.Run(ctx)
}

type chatController struct {
	ctx      context.Context
	api      *clientapi.Client
	baseURL  string
	state    *clientchat.State
	renderer *clientchat.Renderer

	streamMu     sync.Mutex
	streamCancel context.CancelFunc
}

func newChatController(ctx context.Context, api *clientapi.Client, baseURL string, output io.Writer) *chatController {
	return &chatController{
		ctx:      ctx,
		api:      api,
		baseURL:  baseURL,
		state:    clientchat.NewState(),
		renderer: clientchat.NewRenderer(output, "chat> "),
	}
}

func (c *chatController) start(ctx context.Context, prompt string) error {
	request := clientapi.StartRequest{
		UserPrompt: strings.TrimSpace(prompt),
	}
	if request.UserPrompt == "" {
		return errors.New("/start requires prompt text")
	}

	state, _, err := c.api.Start(ctx, request)
	if err != nil {
		return err
	}

	c.state.SetActiveRun(state.RunID)
	c.startStream(state.RunID)
	return writeRunState(c.rendererWriter(), state)
}

func (c *chatController) status(ctx context.Context) error {
	runID, _, ok := c.state.ActiveRun()
	if !ok {
		return errors.New("no active run; use /start first")
	}

	state, _, err := c.api.Get(ctx, runID)
	if err != nil {
		return err
	}

	return writeRunState(c.rendererWriter(), state)
}

func (c *chatController) continueRun(ctx context.Context, maxSteps *int) error {
	runID, _, ok := c.state.ActiveRun()
	if !ok {
		return errors.New("no active run; use /start first")
	}

	state, _, err := c.api.Continue(ctx, runID, clientapi.ContinueRequest{MaxSteps: maxSteps})
	if err != nil {
		return err
	}

	return writeRunState(c.rendererWriter(), state)
}

func (c *chatController) steer(ctx context.Context, instruction string) error {
	runID, _, ok := c.state.ActiveRun()
	if !ok {
		return errors.New("no active run; use /start first")
	}
	if strings.TrimSpace(instruction) == "" {
		return errors.New("/steer requires instruction text")
	}

	state, _, err := c.api.Steer(ctx, runID, clientapi.SteerRequest{
		Instruction: instruction,
	})
	if err != nil {
		return err
	}

	return writeRunState(c.rendererWriter(), state)
}

func (c *chatController) followUp(ctx context.Context, prompt string) error {
	runID, _, ok := c.state.ActiveRun()
	if !ok {
		return errors.New("no active run; use /start first")
	}
	if strings.TrimSpace(prompt) == "" {
		return errors.New("/followup requires prompt text")
	}

	state, _, err := c.api.FollowUp(ctx, runID, clientapi.FollowUpRequest{
		Prompt: prompt,
	})
	if err != nil {
		return err
	}

	return writeRunState(c.rendererWriter(), state)
}

func (c *chatController) cancel(ctx context.Context) error {
	runID, _, ok := c.state.ActiveRun()
	if !ok {
		return errors.New("no active run; use /start first")
	}

	state, _, err := c.api.Cancel(ctx, runID)
	if err != nil {
		return err
	}

	return writeRunState(c.rendererWriter(), state)
}

func (c *chatController) startStream(runID string) {
	c.streamMu.Lock()
	if c.streamCancel != nil {
		c.streamCancel()
		c.streamCancel = nil
	}
	streamCtx, cancel := context.WithCancel(c.ctx)
	c.streamCancel = cancel
	c.streamMu.Unlock()

	go c.streamLoop(streamCtx, runID)
}

func (c *chatController) stopStream() {
	c.streamMu.Lock()
	cancel := c.streamCancel
	c.streamCancel = nil
	c.streamMu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (c *chatController) streamLoop(ctx context.Context, runID string) {
	httpClient := &http.Client{}

	var currentCursor int64
	if activeRunID, cursor, ok := c.state.ActiveRun(); ok && activeRunID == runID {
		currentCursor = cursor
	}

	for {
		streamBody, err := openEventsStream(ctx, httpClient, c.baseURL, runID, currentCursor)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			_ = c.renderer.PrintLine("stream error: " + err.Error())
			return
		}

		reader := clientevents.NewReader(streamBody)
		needReconnect := false
		for {
			frame, _, err := reader.Next()
			if err != nil {
				_ = streamBody.Close()
				if errors.Is(err, io.EOF) {
					needReconnect = true
					break
				}
				_ = c.renderer.PrintLine("stream error: " + err.Error())
				return
			}

			if frame.ID <= currentCursor {
				_ = streamBody.Close()
				_ = c.renderer.PrintLine(
					fmt.Sprintf("stream error: non-monotonic id=%d cursor=%d", frame.ID, currentCursor),
				)
				return
			}
			currentCursor = frame.ID
			c.state.AdvanceCursor(runID, currentCursor)

			if err := c.renderer.PrintLine(formatStreamEvent(frame)); err != nil {
				_ = streamBody.Close()
				return
			}
		}

		if !needReconnect {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(eventsReconnectDelay):
		}
	}
}

func (c *chatController) rendererWriter() io.Writer {
	return lineWriter{renderer: c.renderer}
}

type lineWriter struct {
	renderer *clientchat.Renderer
}

func (w lineWriter) Write(p []byte) (int, error) {
	trimmed := strings.TrimRight(string(p), "\n")
	if err := w.renderer.PrintLine(trimmed); err != nil {
		return 0, err
	}
	return len(p), nil
}

func formatStreamEvent(frame clientevents.StreamEvent) string {
	return fmt.Sprintf(
		"event id=%d run_id=%s step=%d type=%s description=%s",
		frame.ID,
		frame.Event.RunID,
		frame.Event.Step,
		frame.Event.Type,
		strings.TrimSpace(frame.Event.Description),
	)
}
