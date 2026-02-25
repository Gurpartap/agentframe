package tools

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"agentruntime/agent"
)

// Handler executes business logic for one tool call.
type Handler func(ctx context.Context, arguments map[string]any) (string, error)

// Registry is a minimal map-backed tool executor.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func NewRegistry(initial map[string]Handler) *Registry {
	handlers := make(map[string]Handler, len(initial))
	maps.Copy(handlers, initial)
	return &Registry{handlers: handlers}
}

func (r *Registry) Register(name string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
}

func (r *Registry) Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
	r.mu.RLock()
	handler, ok := r.handlers[call.Name]
	r.mu.RUnlock()
	if !ok {
		return agent.ToolResult{}, fmt.Errorf("tool %q is not registered", call.Name)
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
