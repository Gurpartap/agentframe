GO ?= go

SERVER_DIR := examples/coding-agent/server
CLIENT_DIR := examples/coding-agent/client

.PHONY: help verify \
	root root-test root-build root-vet \
	server server-test server-build server-vet \
	client client-test client-build client-vet

help:
	@echo "Targets:"
	@echo "  verify       Run full repo + server + client test/build/vet gate"
	@echo "  root         Run root test/build/vet"
	@echo "  server       Run server test/build/vet"
	@echo "  client       Run client test/build/vet"

verify: root server client

root: root-test root-build root-vet

root-test:
	$(GO) test ./...

root-build:
	$(GO) build ./...

root-vet:
	$(GO) vet ./...

server: server-test server-build server-vet

server-test:
	cd $(SERVER_DIR) && $(GO) test ./...

server-build:
	cd $(SERVER_DIR) && $(GO) build ./...

server-vet:
	cd $(SERVER_DIR) && $(GO) vet ./...

client: client-test client-build client-vet

client-test:
	cd $(CLIENT_DIR) && $(GO) test ./...

client-build:
	cd $(CLIENT_DIR) && $(GO) build ./...

client-vet:
	cd $(CLIENT_DIR) && $(GO) vet ./...
