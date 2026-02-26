package clientchat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var ErrQuit = errors.New("quit chat")

type Handlers struct {
	Start    func(ctx context.Context, prompt string) error
	Status   func(ctx context.Context) error
	Continue func(ctx context.Context, maxSteps *int, resolution *ResolutionInput) error
	Steer    func(ctx context.Context, instruction string) error
	FollowUp func(ctx context.Context, prompt string) error
	Cancel   func(ctx context.Context) error
}

type REPL struct {
	in       *bufio.Reader
	renderer *Renderer
	handlers Handlers
}

func NewREPL(in io.Reader, renderer *Renderer, handlers Handlers) *REPL {
	if in == nil {
		in = strings.NewReader("")
	}
	if renderer == nil {
		renderer = NewRenderer(io.Discard, defaultPrompt)
	}
	return &REPL{
		in:       bufio.NewReader(in),
		renderer: renderer,
		handlers: handlers,
	}
}

func (r *REPL) Run(ctx context.Context) error {
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil
		}

		if err := r.renderer.ShowPrompt(); err != nil {
			return err
		}
		line, err := r.in.ReadString('\n')
		r.renderer.HidePrompt()
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if errors.Is(err, io.EOF) {
				return nil
			}
			continue
		}

		dispatchErr := r.dispatch(ctx, trimmed)
		switch {
		case dispatchErr == nil:
		case errors.Is(dispatchErr, ErrQuit):
			return nil
		default:
			if writeErr := r.renderer.PrintLine("error: " + dispatchErr.Error()); writeErr != nil {
				return writeErr
			}
		}

		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func (r *REPL) dispatch(ctx context.Context, line string) error {
	if !strings.HasPrefix(line, "/") {
		return r.followUp(ctx, line)
	}

	commandWithPrefix := line
	args := ""
	if i := strings.IndexByte(line, ' '); i >= 0 {
		commandWithPrefix = line[:i]
		args = strings.TrimSpace(line[i+1:])
	}
	command := strings.TrimPrefix(commandWithPrefix, "/")

	switch command {
	case "start":
		if args == "" {
			return errors.New("/start requires prompt text")
		}
		if r.handlers.Start == nil {
			return errors.New("start command is not configured")
		}
		return r.handlers.Start(ctx, args)
	case "status":
		if r.handlers.Status == nil {
			return errors.New("status command is not configured")
		}
		return r.handlers.Status(ctx)
	case "continue":
		if r.handlers.Continue == nil {
			return errors.New("continue command is not configured")
		}
		maxSteps, err := parseContinueArgs(args)
		if err != nil {
			return err
		}
		continueErr := r.handlers.Continue(ctx, maxSteps, nil)
		if continueErr == nil {
			return nil
		}

		var resolutionRequiredErr *ResolutionRequiredError
		if !errors.As(continueErr, &resolutionRequiredErr) {
			return continueErr
		}

		resolution, err := PromptResolution(ctx, r.in, r.renderer, resolutionRequiredErr.Defaults())
		if err != nil {
			return err
		}
		return r.handlers.Continue(ctx, maxSteps, resolution)
	case "steer":
		if args == "" {
			return errors.New("/steer requires instruction text")
		}
		if r.handlers.Steer == nil {
			return errors.New("steer command is not configured")
		}
		return r.handlers.Steer(ctx, args)
	case "followup":
		if args == "" {
			return errors.New("/followup requires prompt text")
		}
		return r.followUp(ctx, args)
	case "cancel":
		if r.handlers.Cancel == nil {
			return errors.New("cancel command is not configured")
		}
		return r.handlers.Cancel(ctx)
	case "quit":
		return ErrQuit
	default:
		return fmt.Errorf("unsupported command %q", commandWithPrefix)
	}
}

func (r *REPL) followUp(ctx context.Context, prompt string) error {
	if r.handlers.FollowUp == nil {
		return errors.New("follow-up command is not configured")
	}
	return r.handlers.FollowUp(ctx, prompt)
}

func parseContinueArgs(args string) (*int, error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return nil, nil
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 1 {
		value, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, errors.New("/continue expects an integer max-steps or --max-steps <n>")
		}
		if value <= 0 {
			return nil, errors.New("/continue max-steps must be > 0")
		}
		return &value, nil
	}

	if len(fields) == 2 && fields[0] == "--max-steps" {
		value, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, errors.New("/continue --max-steps expects an integer")
		}
		if value <= 0 {
			return nil, errors.New("/continue max-steps must be > 0")
		}
		return &value, nil
	}

	return nil, errors.New("/continue accepts no args, <max-steps>, or --max-steps <n>")
}
