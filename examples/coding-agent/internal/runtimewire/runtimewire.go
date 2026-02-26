package runtimewire

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
	eventinginmem "github.com/Gurpartap/agentframe/eventing/inmem"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runstream"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runtimewire/mocks"
)

// Runtime contains the composed runtime dependencies for the server.
type Runtime struct {
	Runner          *agent.Runner
	RunStore        *runstoreinmem.Store
	EventSink       *eventinginmem.Sink
	StreamBroker    *runstream.Broker
	ToolDefinitions []agent.ToolDefinition
}

func New() (*Runtime, error) {
	store := runstoreinmem.New()
	events := eventinginmem.New()
	streamBroker := runstream.New(runstream.DefaultHistoryLimit)
	fanout := newFanoutSink(events, streamBroker)

	model := mocks.NewModel()
	tools := mocks.NewTools()
	loop, err := agentreact.New(model, tools, fanout)
	if err != nil {
		return nil, fmt.Errorf("new runtime loop: %w", err)
	}

	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newSequenceIDGenerator(),
		RunStore:    store,
		Engine:      loop,
		EventSink:   fanout,
	})
	if err != nil {
		return nil, fmt.Errorf("new runtime runner: %w", err)
	}

	return &Runtime{
		Runner:          runner,
		RunStore:        store,
		EventSink:       events,
		StreamBroker:    streamBroker,
		ToolDefinitions: mocks.Definitions(),
	}, nil
}

type fanoutSink struct {
	sinks []agent.EventSink
}

func newFanoutSink(sinks ...agent.EventSink) fanoutSink {
	filtered := make([]agent.EventSink, 0, len(sinks))
	for _, sink := range sinks {
		if sink != nil {
			filtered = append(filtered, sink)
		}
	}
	return fanoutSink{sinks: filtered}
}

func (s fanoutSink) Publish(ctx context.Context, event agent.Event) error {
	var result error
	for _, sink := range s.sinks {
		if err := sink.Publish(ctx, event); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
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
