SHELL := /bin/zsh

.PHONY: bootstrap ensure-cliproxy cliproxy-check cliproxy-update dev backend frontend test build proxy-status proxy-start proxy-stop proxy-restart

API_BASE := http://127.0.0.1:18417

bootstrap: ensure-cliproxy

ensure-cliproxy:
	@./scripts/ensure_cliproxyapi.sh

cliproxy-check:
	@./scripts/ensure_cliproxyapi.sh --check

cliproxy-update:
	@set -e; \
	if [[ -n "$(VERSION)" ]]; then \
		./scripts/ensure_cliproxyapi.sh --update --version "$(VERSION)"; \
	else \
		./scripts/ensure_cliproxyapi.sh --update --latest; \
	fi

dev: ensure-cliproxy
	@set -e; \
		(go run ./cmd/server) & \
		BACKEND_PID=$$!; \
		(cd web && corepack pnpm dev --host 127.0.0.1 --port 5173) & \
		FRONTEND_PID=$$!; \
		cleanup() { \
			kill $$BACKEND_PID $$FRONTEND_PID 2>/dev/null || true; \
			wait $$BACKEND_PID $$FRONTEND_PID 2>/dev/null || true; \
		}; \
		trap cleanup INT TERM EXIT; \
		while true; do \
			if ! kill -0 $$BACKEND_PID 2>/dev/null; then break; fi; \
			if ! kill -0 $$FRONTEND_PID 2>/dev/null; then break; fi; \
			sleep 1; \
		done

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

proxy-status:
	@curl -sS "$(API_BASE)/api/proxy/status"; echo

proxy-start:
	@curl -sS -X POST "$(API_BASE)/api/proxy/start"; echo

proxy-stop:
	@curl -sS -X POST "$(API_BASE)/api/proxy/stop"; echo

proxy-restart:
	@curl -sS -X POST "$(API_BASE)/api/proxy/restart"; echo
