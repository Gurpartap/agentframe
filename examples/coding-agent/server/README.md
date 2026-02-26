# Coding Agent Server

This app root hosts the HTTP runtime server for coding-agent runs.

For CLI usage and interactive workflows, see [../client/README.md](../client/README.md).

## Run

Run from `examples/coding-agent/server`:

```bash
go run ./cmd/server
```

Default listen address: `127.0.0.1:8080`

## Runtime Behavior

- Run store is in-memory.
- Event history is buffered in-memory per run.
- Model mode is selected by `CODING_AGENT_MODEL_MODE`.
- Tool mode is selected by `CODING_AGENT_TOOL_MODE`.
- Real tool mode exposes exactly `read`, `write`, `edit`, `bash`.

## Configuration

| ENV VAR | Default / behavior |
|---|---|
| `CODING_AGENT_HTTP_ADDR` | `127.0.0.1:8080` |
| `CODING_AGENT_SHUTDOWN_TIMEOUT` | `5s` |
| `CODING_AGENT_LOG_LEVEL` | `info` (`debug`, `info`, `warn`, `error`) |
| `CODING_AGENT_LOG_FORMAT` | `text` (`text` or `json`) |
| `CODING_AGENT_MODEL_MODE` | `mock` (`mock` or `provider`) |
| `CODING_AGENT_PROVIDER_API_KEY` | required in `provider` mode |
| `CODING_AGENT_PROVIDER_MODEL` | `gpt-4.1-mini` |
| `CODING_AGENT_PROVIDER_BASE_URL` | `https://api.openai.com/v1` |
| `CODING_AGENT_PROVIDER_TIMEOUT` | `30s` |
| `CODING_AGENT_TOOL_MODE` | `real` (`mock` or `real`) |
| `CODING_AGENT_WORKSPACE_ROOT` | process working directory |
| `CODING_AGENT_BASH_TIMEOUT` | `3s` |

Use `CODING_AGENT_LOG_LEVEL=debug` when you want detailed run and event diagnostics in server logs.

## Health Endpoints

- `GET /healthz` -> `200 ok`
- `GET /readyz` -> `200 ready` when booted

## API Surface

Auth token for mutating routes defaults to:

```text
Authorization: Bearer coding-agent-dev-token
```

Mutating routes:

- `POST /v1/runs/start`
- `POST /v1/runs/{run_id}/continue`
- `POST /v1/runs/{run_id}/cancel`
- `POST /v1/runs/{run_id}/steer`
- `POST /v1/runs/{run_id}/follow-up`

Read routes:

- `GET /v1/runs/{run_id}`
- `GET /v1/runs/{run_id}/events?cursor=<n>`

Policy defaults on mutating routes:

- Max request body: `1 MiB`
- Request timeout: `10s`
- Max command steps: `8`

Event stream format:

- `GET /v1/runs/{run_id}/events` uses `application/x-ndjson`.
- Each line is a JSON object with an incremental `id` and the event payload.

## Quick Smoke

```bash
BASE="http://127.0.0.1:8080"
TOKEN="coding-agent-dev-token"

curl -sS "$BASE/healthz"
curl -sS "$BASE/readyz"

curl -sS -X POST "$BASE/v1/runs/start" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_prompt":"Create notes.txt with hello","max_steps":4}'
```

## Verify

From `examples/coding-agent/server`:

```bash
go test ./...
go build ./...
go vet ./...
```
