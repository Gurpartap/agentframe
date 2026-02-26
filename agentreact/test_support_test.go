package agentreact_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
	eventinginmem "github.com/Gurpartap/agentframe/eventing/inmem"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"
	toolingregistry "github.com/Gurpartap/agentframe/tooling/registry"
)

type response struct {
	Message agent.Message
	Err     error
}

type scriptedModel struct {
	mu        sync.Mutex
	index     int
	responses []response
}

func newScriptedModel(responses ...response) *scriptedModel {
	cloned := make([]response, len(responses))
	copy(cloned, responses)
	return &scriptedModel{responses: cloned}
}

var _ agentreact.Model = (*scriptedModel)(nil)

func (m *scriptedModel) Generate(_ context.Context, _ agentreact.ModelRequest) (agent.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.index >= len(m.responses) {
		return agent.Message{}, fmt.Errorf("script exhausted at step %d", m.index+1)
	}
	current := m.responses[m.index]
	m.index++
	if current.Err != nil {
		return agent.Message{}, current.Err
	}
	msg := agent.CloneMessage(current.Message)
	if msg.Role == "" {
		msg.Role = agent.RoleAssistant
	}
	return msg, nil
}

type handler = toolingregistry.Handler
type registry = toolingregistry.Registry

func newRegistry(initial map[string]handler) *registry {
	registry, err := toolingregistry.New(initial)
	if err != nil {
		panic(err)
	}
	return registry
}

type runStore = runstoreinmem.Store

func newRunStore() *runStore {
	return runstoreinmem.New()
}

type eventSink = eventinginmem.Sink

func newEventSink() *eventSink {
	return eventinginmem.New()
}

type counterIDGenerator struct {
	prefix  string
	counter atomic.Uint64
}

func newCounterIDGenerator(prefix string) *counterIDGenerator {
	if prefix == "" {
		prefix = "run"
	}
	return &counterIDGenerator{prefix: prefix}
}

func (g *counterIDGenerator) NewRunID(_ context.Context) (agent.RunID, error) {
	next := g.counter.Add(1)
	return agent.RunID(fmt.Sprintf("%s-%06d", g.prefix, next)), nil
}
