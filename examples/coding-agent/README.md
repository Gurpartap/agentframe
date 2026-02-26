# Coding Agent Example

`coding-agent` contains two independent apps:

- `server/`: HTTP runtime server
- `client/`: terminal CLI for run control and chat flows

## App-Specific Guides

- Server guide: [server/README.md](./server/README.md)
- Client guide: [client/README.md](./client/README.md)

## Verify

From `examples/coding-agent`:

```bash
go test ./...
go build ./...
go vet ./...
```
