# Coding Agent Example

This package runs an HTTP server that hosts a coding-oriented agent runtime.

## Runtime Shape

- Run store is in-memory.
- Stream events are buffered in-memory per run.
- One model provider is active per process, selected by `CODING_AGENT_MODEL_MODE`.
- Tool mode is selected by `CODING_AGENT_TOOL_MODE`.
- Real tool mode exposes exactly `read`, `write`, `edit`, and `bash`.

## Quick Start

From `examples/coding-agent`:

```bash
go run ./cmd/server
```

From the repository root:

```bash
go run ./examples/coding-agent/cmd/server
```

Default listen address: `127.0.0.1:8080`

`CODING_AGENT_WORKSPACE_ROOT` defaults to the current working directory of the server process.

Health endpoints:

- `GET /healthz` -> `200 ok`
- `GET /readyz` -> `200 ready` when booted

## Configuration
| ENV VAR | Default / behavior |
|---|---|
| `CODING_AGENT_HTTP_ADDR` | `127.0.0.1:8080` |
| `CODING_AGENT_SHUTDOWN_TIMEOUT` | `5s` |
| `CODING_AGENT_MODEL_MODE` | `mock` (values: `mock` or `provider`) |
| `CODING_AGENT_PROVIDER_API_KEY` | required in `provider` mode |
| `CODING_AGENT_PROVIDER_MODEL` | `gpt-4.1-mini` |
| `CODING_AGENT_PROVIDER_BASE_URL` | `https://api.openai.com/v1` |
| `CODING_AGENT_PROVIDER_TIMEOUT` | `30s` |
| `CODING_AGENT_TOOL_MODE` | `real` (values: `mock` or `real`) |
| `CODING_AGENT_WORKSPACE_ROOT` | process working directory |
| `CODING_AGENT_BASH_TIMEOUT` | `3s` |

## HTTP API

Mutating routes are auth-protected. The default token is static:

```text
Authorization: Bearer coding-agent-dev-token
```

Mutating routes:

```
POST /v1/runs/start
POST /v1/runs/{run_id}/continue
POST /v1/runs/{run_id}/cancel
POST /v1/runs/{run_id}/steer
POST /v1/runs/{run_id}/follow-up
```

Read routes (no auth middleware):

```
GET /v1/runs/{run_id}
GET /v1/runs/{run_id}/events?cursor=<n>
```

`GET /v1/runs/{run_id}/events` streams `application/x-ndjson` with one JSON object per line:

```json
{"id":1,"event":{"run_id":"run-000001","step":0,"type":"run_started"}}
```

Policy defaults on mutating routes:

- Request body limit: `1 MiB` (`1048576` bytes)
- Request timeout: `10s`
- Max command steps: `8`

## Command Route Examples

Example environment:

```bash
TOKEN="coding-agent-dev-token"
BASE="http://127.0.0.1:8080"
RUN_ID="run-000001"
```

#### `POST /v1/runs/start`

```bash
curl -sS -X POST "$BASE/v1/runs/start" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_prompt":"Create notes.txt with hello","max_steps":4}'
```

```json
{
  "run_id": "run-000001",
  "status": "completed",
  "step": 2,
  "version": 3,
  "output": "done"
}
```

#### `POST /v1/runs/{run_id}/continue` (without resolution)

```bash
curl -sS -X POST "$BASE/v1/runs/$RUN_ID/continue" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"max_steps":4}'
```

```json
{
  "run_id": "run-000001",
  "status": "running",
  "step": 3,
  "version": 4
}
```

#### `POST /v1/runs/{run_id}/continue` (with resolution)

```bash
curl -sS -X POST "$BASE/v1/runs/$RUN_ID/continue" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"max_steps":4,"resolution":{"requirement_id":"req-1","kind":"approval","outcome":"approved","value":"ok"}}'
```

```json
{
  "run_id": "run-000001",
  "status": "completed",
  "step": 4,
  "version": 5,
  "output": "done"
}
```

#### `POST /v1/runs/{run_id}/cancel`

```bash
curl -sS -X POST "$BASE/v1/runs/$RUN_ID/cancel" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "run_id": "run-000001",
  "status": "cancelled",
  "step": 3,
  "version": 4
}
```

#### `POST /v1/runs/{run_id}/steer`

```bash
curl -sS -X POST "$BASE/v1/runs/$RUN_ID/steer" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"instruction":"Use shorter output."}'
```

```json
{
  "run_id": "run-000001",
  "status": "running",
  "step": 3,
  "version": 4
}
```

#### `POST /v1/runs/{run_id}/follow-up`

```bash
curl -sS -X POST "$BASE/v1/runs/$RUN_ID/follow-up" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"Now add a summary section.","max_steps":3}'
```

```json
{
  "run_id": "run-000001",
  "status": "completed",
  "step": 4,
  "version": 5,
  "output": "updated summary"
}
```

#### `GET /v1/runs/{run_id}`

```bash
curl -sS "$BASE/v1/runs/$RUN_ID"
```

```json
{
  "run_id": "run-000001",
  "status": "completed",
  "step": 4,
  "version": 5,
  "output": "updated summary"
}
```

#### `GET /v1/runs/{run_id}/events`

```bash
curl -N -sS "$BASE/v1/runs/$RUN_ID/events"
```

```text
{"id":1,"event":{"run_id":"run-000001","step":0,"type":"run_started"}}
```

#### `GET /v1/runs/{run_id}/events?cursor=<n>`

```bash
curl -N -sS "$BASE/v1/runs/$RUN_ID/events?cursor=12"
```

```text
{"id":13,"event":{"run_id":"run-000001","step":4,"type":"run_completed","description":"run completed"}}
```

Unauthorized mutating request (missing or invalid bearer token):

```bash
# Missing bearer token
curl -sS -X POST "$BASE/v1/runs/start" \
  -H "Content-Type: application/json" \
  -d '{"user_prompt":"Create notes.txt with hello"}'

# Invalid bearer token
curl -sS -X POST "$BASE/v1/runs/start" \
  -H "Authorization: Bearer wrong-token" \
  -H "Content-Type: application/json" \
  -d '{"user_prompt":"Create notes.txt with hello"}'
```

```json
{
  "error": {
    "code": "unauthorized",
    "message": "policy authentication failed: missing or invalid bearer token"
  }
}
```

Optional response fields:

- `error` appears when the run is failed.
- `pending_requirement` appears when the run is suspended.

## Validation

```bash
go build ./...
go test ./...
```
