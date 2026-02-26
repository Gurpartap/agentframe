# Coding Agent Client

This app root hosts the CLI for controlling coding-agent runs.

## Start The Server

Start the server using the instructions in [../server/README.md](../server/README.md).

Default server address: `http://127.0.0.1:8080`

## Run The Client

All commands below assume your current directory is `examples/coding-agent/client`.

```bash
go run ./cmd/client --help
```

Global flags:

- `--base-url` (default `http://127.0.0.1:8080`)
- `--token` (default `coding-agent-dev-token`)
- `--json` (print raw API payload)
- `--timeout` (default `15s`)

## Chat Mode

Start chat:

```bash
go run ./cmd/client chat
```

Slash commands:

- `/start <prompt>`
- `/status`
- `/continue [--max-steps <n>]`
- `/steer <instruction>`
- `/followup <prompt>`
- `/cancel`
- `/quit`

Free text is treated as `/followup <text>` on the active run.

If a run is suspended, `/continue` prompts for:

1. `requirement_id`
2. `kind` (`approval`, `user_input`, `external_execution`)
3. `outcome` (`approved`, `rejected`, `provided`, `completed`)
4. optional `value`

The client then submits a typed resolution payload through `continue`.

## Non-Interactive Commands

Health:

```bash
go run ./cmd/client health
```

Start:

```bash
go run ./cmd/client start --user-prompt "Create notes.txt with hello"
```

Get:

```bash
go run ./cmd/client get run-000001
```

Stream events from the beginning:

```bash
go run ./cmd/client events run-000001 --cursor 0
```

Continue without resolution:

```bash
go run ./cmd/client continue run-000001 --max-steps 2
```

Continue with typed resolution:

```bash
go run ./cmd/client continue run-000001 \
  --max-steps 2 \
  --requirement-id req-approval \
  --kind approval \
  --outcome approved
```

Continue with typed resolution and value:

```bash
go run ./cmd/client continue run-000001 \
  --requirement-id req-user-input \
  --kind user_input \
  --outcome provided \
  --value "operator supplied value"
```

Steer:

```bash
go run ./cmd/client steer run-000001 --instruction "Use a shorter plan."
```

Follow-up:

```bash
go run ./cmd/client follow-up run-000001 --prompt "Now summarize the changes."
```

Cancel:

```bash
go run ./cmd/client cancel run-000001
```

## Troubleshooting

- `error: no active run; use /start first`: start a run before `/status`, `/continue`, `/steer`, `/followup`, or `/cancel`.
- `continue resolution kind: unsupported requirement kind ...`: use one of `approval`, `user_input`, `external_execution`.
- `continue resolution outcome: unsupported resolution outcome ...`: use one of `approved`, `rejected`, `provided`, `completed`.
- `events stream rejected ... code=conflict message=cursor expired`: reconnect with a newer cursor or restart from `--cursor 0`.
- Unauthorized errors on mutating commands: check `--token` and server auth policy.

## Verify

From `examples/coding-agent/client`:

```bash
go test ./...
go build ./...
go vet ./...
```
