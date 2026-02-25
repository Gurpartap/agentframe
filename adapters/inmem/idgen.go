package inmem

import (
	"context"
	"fmt"
	"sync/atomic"

	"agentruntime/agent"
)

// CounterIDGenerator provides deterministic in-process run IDs.
type CounterIDGenerator struct {
	prefix  string
	counter atomic.Uint64
}

func NewCounterIDGenerator(prefix string) *CounterIDGenerator {
	if prefix == "" {
		prefix = "run"
	}
	return &CounterIDGenerator{
		prefix: prefix,
	}
}

func (g *CounterIDGenerator) NewRunID(_ context.Context) (agent.RunID, error) {
	next := g.counter.Add(1)
	return agent.RunID(fmt.Sprintf("%s-%06d", g.prefix, next)), nil
}
