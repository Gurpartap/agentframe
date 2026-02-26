# agentframe

Concrete package skeleton for an agent runtime in Go with domain-first boundaries.

- `agent`: runtime core contracts and command/lifecycle semantics.
- `agentreact`: ReAct engine implementation built on top of `agent` contracts.
- `policy/retry`: optional retry wrappers for model/tool execution.

Layering still exists, but it is represented by file-level boundaries inside `agent` instead of generic package names.

## ReAct loop behavior

1. Load transcript and tool definitions.
2. Ask model for next assistant message.
3. If no tool calls, finish run.
4. Execute tool calls and append tool observation messages.
5. Repeat until completion or `maxSteps`.

## Shared wiring

```go
import (
	"context"
	"fmt"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
	eventinginmem "github.com/Gurpartap/agentframe/eventing/inmem"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"
	toolingregistry "github.com/Gurpartap/agentframe/tooling/registry"
)

type staticIDGenerator struct{}

func (staticIDGenerator) NewRunID(context.Context) (agent.RunID, error) {
	return "generated-run-id", nil
}

type scriptedModel struct {
	messages []agent.Message
	index    int
}

func (m *scriptedModel) Generate(_ context.Context, _ agentreact.ModelRequest) (agent.Message, error) {
	if m.index >= len(m.messages) {
		return agent.Message{}, fmt.Errorf("script exhausted")
	}
	msg := m.messages[m.index]
	m.index++
	if msg.Role == "" {
		msg.Role = agent.RoleAssistant
	}
	return msg, nil
}

func newRunner(model agentreact.Model) (*agent.Runner, *runstoreinmem.Store, error) {
	store := runstoreinmem.New()
	events := eventinginmem.New()
	tools, err := toolingregistry.New(map[string]toolingregistry.Handler{})
	if err != nil {
		return nil, nil, err
	}
	engine, err := agentreact.New(model, tools, events)
	if err != nil {
		return nil, nil, err
	}
	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: staticIDGenerator{},
		RunStore:    store,
		Engine:      engine,
		EventSink:   events,
	})
	if err != nil {
		return nil, nil, err
	}
	return runner, store, nil
}
```

## How-to: run to completion

```go
ctx := context.Background()

model := &scriptedModel{
	messages: []agent.Message{
		{Content: "done"},
	},
}
runner, _, err := newRunner(model)
if err != nil {
	panic(err)
}

result, err := runner.Run(ctx, agent.RunInput{
	RunID:      "run-quickstart",
	UserPrompt: "Solve task",
	MaxSteps:   4,
})
if err != nil {
	panic(err)
}
fmt.Println(result.State.Status) // completed
fmt.Println(result.State.Output) // done
```

## How-to: suspend then continue with typed resolution

```go
ctx := context.Background()

model := &scriptedModel{
	messages: []agent.Message{
		{
			Content: "approval required",
			Requirement: &agent.PendingRequirement{
				ID:     "req-approval-1",
				Kind:   agent.RequirementKindApproval,
				Prompt: "Approve this run",
			},
		},
		{Content: "approved and completed"},
	},
}
runner, _, err := newRunner(model)
if err != nil {
	panic(err)
}

runResult, err := runner.Run(ctx, agent.RunInput{
	RunID:      "run-suspend-continue",
	UserPrompt: "Start flow",
	MaxSteps:   4,
})
if err != nil {
	panic(err)
}
fmt.Println(runResult.State.Status) // suspended

continued, err := runner.Continue(ctx, runResult.State.ID, 4, nil, &agent.Resolution{
	RequirementID: "req-approval-1",
	Kind:          agent.RequirementKindApproval,
	Outcome:       agent.ResolutionOutcomeApproved,
})
if err != nil {
	panic(err)
}
fmt.Println(continued.State.Status) // completed
```

## How-to: misuse gating (hard errors)

```go
import "errors"

ctx := context.Background()

model := &scriptedModel{
	messages: []agent.Message{
		{
			Content: "approval required",
			Requirement: &agent.PendingRequirement{
				ID:   "req-1",
				Kind: agent.RequirementKindApproval,
			},
		},
	},
}
runner, store, err := newRunner(model)
if err != nil {
	panic(err)
}

suspended, err := runner.Run(ctx, agent.RunInput{
	RunID:      "run-suspended",
	UserPrompt: "Start flow",
	MaxSteps:   2,
})
if err != nil {
	panic(err)
}

_, err = runner.Continue(ctx, suspended.State.ID, 2, nil, nil)
fmt.Println(errors.Is(err, agent.ErrResolutionRequired)) // true

_, err = runner.Continue(ctx, suspended.State.ID, 2, nil, &agent.Resolution{
	RequirementID: "wrong-id",
	Kind:          agent.RequirementKindApproval,
	Outcome:       agent.ResolutionOutcomeApproved,
})
fmt.Println(errors.Is(err, agent.ErrResolutionInvalid)) // true

_, err = runner.Steer(ctx, suspended.State.ID, "new instruction")
fmt.Println(errors.Is(err, agent.ErrResolutionRequired)) // true

_, err = runner.FollowUp(ctx, suspended.State.ID, "continue", 2, nil)
fmt.Println(errors.Is(err, agent.ErrResolutionRequired)) // true

if err := store.Save(ctx, agent.RunState{
	ID:     "run-open",
	Status: agent.RunStatusPending,
}); err != nil {
	panic(err)
}
_, err = runner.Continue(ctx, "run-open", 2, nil, &agent.Resolution{
	RequirementID: "req-1",
	Kind:          agent.RequirementKindApproval,
	Outcome:       agent.ResolutionOutcomeApproved,
})
fmt.Println(errors.Is(err, agent.ErrResolutionUnexpected)) // true
```

## License
MIT © 2026 Gurpartap Singh (https://x.com/Gurpartap)
