# agentruntime

Concrete package skeleton for an agent runtime in Go with domain-first boundaries.

- `agent`: runtime domain API and logic (types, interfaces, ReAct loop, runner).
- `adapters`: infrastructure/test adapters (`inmem`, `tools`, `modeltest`).

Layering still exists, but it is represented by file-level boundaries inside `agent` instead of generic package names.

## ReAct loop behavior

1. Load transcript and tool definitions.
2. Ask model for next assistant message.
3. If no tool calls, finish run.
4. Execute tool calls and append tool observation messages.
5. Repeat until completion or `maxSteps`.

## Quick wiring

```go
model := /* your agent.Model implementation */
tools := /* your agent.ToolExecutor implementation */
events := inmem.NewEventSink()
loop, _ := agent.NewReactLoop(model, tools, events)

runner, _ := agent.NewRunner(agent.Dependencies{
	IDGenerator: inmem.NewCounterIDGenerator("run"),
	RunStore:    inmem.NewRunStore(),
	ReactLoop:   loop,
	EventSink:   events,
})

result, err := runner.Run(ctx, agent.RunInput{
	UserPrompt: "Solve task",
	MaxSteps:   8,
	Tools:      []agent.ToolDefinition{{Name: "search"}},
})
```
