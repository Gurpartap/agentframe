# Coding Agent Example

This package runs an HTTP server that hosts a coding-oriented agent runtime.

## Runtime Shape

- Run store is in-memory.
- Stream events are buffered in-memory per run.
- One model provider is active per process, selected by `CODING_AGENT_MODEL_MODE`.
- Tool mode is selected by `CODING_AGENT_TOOL_MODE`.
- Real tool mode exposes exactly `read`, `write`, `edit`, and `bash`.

## Quick Start

```bash
go run ./cmd/server
```

Default listen address: `127.0.0.1:8080`

Health endpoints:

- `GET /healthz` -> `200 ok`
- `GET /readyz` -> `200 ready` when booted

## Configuration
| ENV VAR | Default value |
|---|---|
| `CODING_AGENT_HTTP_ADDR` | `127.0.0.1:8080` |
| `CODING_AGENT_SHUTDOWN_TIMEOUT` | `5s` |
| `CODING_AGENT_MODEL_MODE` | `mock` (values: `mock` or `provider`) |
| `CODING_AGENT_PROVIDER_API_KEY` | required in `provider` mode |
| `CODING_AGENT_PROVIDER_MODEL` | required in `provider` mode |
| `CODING_AGENT_PROVIDER_BASE_URL` | required in `provider` mode |
| `CODING_AGENT_PROVIDER_TIMEOUT` | `30s` |
| `CODING_AGENT_TOOL_MODE` | `real` (values: `mock` or `real`) |
| `CODING_AGENT_WORKSPACE_ROOT` | process working directory |
| `CODING_AGENT_BASH_TIMEOUT` | `3s` |

## HTTP API

Mutating routes require:

```text
Authorization: Bearer coding-agent-dev-token
```

Routes:

```
POST /v1/runs/start
POST /v1/runs/{run_id}/continue
POST /v1/runs/{run_id}/cancel
POST /v1/runs/{run_id}/steer
POST /v1/runs/{run_id}/follow-up
GET /v1/runs/{run_id}
GET /v1/runs/{run_id}/events?cursor=<n>
```

## Validation

```bash
go build ./...
go test ./...
```
