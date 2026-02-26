package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Gurpartap/agentframe/agent"
)

var (
	ErrToolUnregistered = errors.New("tool is not registered")
	ErrNilHandler       = errors.New("tool handler is nil")
	ErrToolNameEmpty    = errors.New("tool name is empty")
)

// Handler executes one tool call using parsed arguments.
type Handler func(ctx context.Context, arguments map[string]any) (string, error)

// Registry stores handlers by tool name and executes tool calls.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func New(initial map[string]Handler) (*Registry, error) {
	handlers := make(map[string]Handler, len(initial))
	for name, handler := range initial {
		if err := validateRegistration(name, handler); err != nil {
			return nil, err
		}
		handlers[name] = handler
	}
	return &Registry{handlers: handlers}, nil
}

func (r *Registry) Register(name string, handler Handler) error {
	if err := validateRegistration(name, handler); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
	return nil
}

func validateRegistration(name string, handler Handler) error {
	if name == "" {
		return ErrToolNameEmpty
	}
	if handler == nil {
		return ErrNilHandler
	}
	return nil
}

func (r *Registry) Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
	if ctx == nil {
		return agent.ToolResult{}, agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return agent.ToolResult{}, ctxErr
	}
	if call.Name == "" {
		return agent.ToolResult{}, fmt.Errorf("%w: call %q", ErrToolNameEmpty, call.ID)
	}

	r.mu.RLock()
	handler, ok := r.handlers[call.Name]
	r.mu.RUnlock()
	if !ok {
		return agent.ToolResult{}, fmt.Errorf("%w: %q", ErrToolUnregistered, call.Name)
	}
	if handler == nil {
		return agent.ToolResult{}, fmt.Errorf("%w: %q", ErrNilHandler, call.Name)
	}

	content, err := handler(ctx, call.Arguments)
	if err != nil {
		return agent.ToolResult{}, err
	}

	return agent.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: content,
	}, nil
}
