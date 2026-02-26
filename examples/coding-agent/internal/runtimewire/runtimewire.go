package runtimewire

import (
	"context"
	"fmt"
	"sync"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
	eventinginmem "github.com/Gurpartap/agentframe/eventing/inmem"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runtimewire/mocks"
)

// Runtime contains the composed runtime dependencies for the server.
type Runtime struct {
	Runner          *agent.Runner
	RunStore        *runstoreinmem.Store
	EventSink       *eventinginmem.Sink
	ToolDefinitions []agent.ToolDefinition
}

func New() (*Runtime, error) {
	store := runstoreinmem.New()
	events := eventinginmem.New()

	model := mocks.NewModel()
	tools := mocks.NewTools()
	loop, err := agentreact.New(model, tools, events)
	if err != nil {
		return nil, fmt.Errorf("new runtime loop: %w", err)
	}

	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newSequenceIDGenerator(),
		RunStore:    store,
		Engine:      loop,
		EventSink:   events,
	})
	if err != nil {
		return nil, fmt.Errorf("new runtime runner: %w", err)
	}

	return &Runtime{
		Runner:          runner,
		RunStore:        store,
		EventSink:       events,
		ToolDefinitions: mocks.Definitions(),
	}, nil
}

type sequenceIDGenerator struct {
	mu   sync.Mutex
	next uint64
}

func newSequenceIDGenerator() *sequenceIDGenerator {
	return &sequenceIDGenerator{}
}

func (g *sequenceIDGenerator) NewRunID(_ context.Context) (agent.RunID, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.next++
	return agent.RunID(fmt.Sprintf("run-%06d", g.next)), nil
}
