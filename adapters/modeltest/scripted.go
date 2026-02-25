package modeltest

import (
	"context"
	"fmt"
	"sync"

	"agentruntime/agent"
)

// Response configures one model turn in a scripted sequence.
type Response struct {
	Message agent.Message
	Err     error
}

// ScriptedModel is a deterministic model adapter for runtime tests.
type ScriptedModel struct {
	mu        sync.Mutex
	index     int
	responses []Response
}

func NewScriptedModel(responses ...Response) *ScriptedModel {
	cloned := make([]Response, len(responses))
	copy(cloned, responses)
	return &ScriptedModel{
		responses: cloned,
	}
}

var _ agent.Model = (*ScriptedModel)(nil)

func (m *ScriptedModel) Generate(_ context.Context, _ agent.ModelRequest) (agent.Message, error) {
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
