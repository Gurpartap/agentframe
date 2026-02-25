# agentruntime

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

## Quick wiring

```go
import (
	"agentruntime/agent"
	"agentruntime/agentreact"
)

model := /* your agent.Model implementation */
tools := /* your agent.ToolExecutor implementation */
events := /* your agent.EventSink implementation */
engine, _ := agentreact.New(model, tools, events)

runner, _ := agent.NewRunner(agent.Dependencies{
	IDGenerator: /* your agent.IDGenerator implementation */,
	RunStore:    /* your agent.RunStore implementation */,
	Engine:      engine,
	EventSink:   events,
})

result, err := runner.Run(ctx, agent.RunInput{
	UserPrompt: "Solve task",
	MaxSteps:   8,
	Tools:      []agent.ToolDefinition{{Name: "search"}},
})
```
