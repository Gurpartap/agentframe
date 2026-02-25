package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"agentruntime/agent"
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

func New(initial map[string]Handler) *Registry {
	handlers := make(map[string]Handler, len(initial))
	for name, handler := range initial {
		handlers[name] = handler
	}
	return &Registry{handlers: handlers}
}

func (r *Registry) Register(name string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
}

func (r *Registry) Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
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
