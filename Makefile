SHELL := /bin/zsh

.PHONY: bootstrap ensure-cliproxy dev backend frontend test build

bootstrap: ensure-cliproxy

ensure-cliproxy:
	@./scripts/ensure_cliproxyapi.sh

dev: ensure-cliproxy
	@trap 'kill 0' INT TERM EXIT; \
		(go run ./cmd/server) & \
		(cd web && corepack pnpm dev --host 127.0.0.1 --port 5173) & \
		wait

backend: ensure-cliproxy
	go run ./cmd/server

frontend:
	cd web && corepack pnpm dev --host 127.0.0.1 --port 5173

test:
	go test ./cmd/... ./internal/...
	cd web && corepack pnpm build

build:
	go build ./cmd/server
	cd web && corepack pnpm build
