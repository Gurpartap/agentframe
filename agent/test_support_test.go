package agent_test

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Gurpartap/agentframe/agent"
)

type response struct {
	Message agent.Message
	Err     error
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
