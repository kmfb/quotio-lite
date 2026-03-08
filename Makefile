SHELL := /bin/zsh

.PHONY: bootstrap ensure-cliproxy cliproxy-check cliproxy-update dev backend frontend test build proxy-status proxy-start proxy-stop proxy-restart service-install service-start service-stop service-restart service-status service-logs service-uninstall

API_BASE ?= http://127.0.0.1:18417
PNPM ?= pnpm
GOCACHE ?= $(CURDIR)/.gocache
WEB_NODE_OPTIONS ?= --use-bundled-ca
WEB_BUILD := if [[ -f ./node_modules/typescript/bin/tsc && -f ./node_modules/vite/bin/vite.js ]]; then NODE_OPTIONS=$(WEB_NODE_OPTIONS) node ./node_modules/typescript/bin/tsc -b && NODE_OPTIONS=$(WEB_NODE_OPTIONS) node ./node_modules/vite/bin/vite.js build; else NODE_OPTIONS=$(WEB_NODE_OPTIONS) $(PNPM) build; fi
SERVICE_SCRIPT := ./scripts/launchd_service.sh
BUILD_DIR := build
BUILD_BIN := $(BUILD_DIR)/quotio-lite

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
		(GOCACHE=$(GOCACHE) go run ./cmd/server) & \
		BACKEND_PID=$$!; \
		(cd web && NODE_OPTIONS=$(WEB_NODE_OPTIONS) $(PNPM) dev --host 127.0.0.1 --port 5173) & \
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
	GOCACHE=$(GOCACHE) go run ./cmd/server

frontend:
	cd web && NODE_OPTIONS=$(WEB_NODE_OPTIONS) $(PNPM) dev --host 127.0.0.1 --port 5173

test:
	mkdir -p $(GOCACHE)
	GOCACHE=$(GOCACHE) go test ./cmd/... ./internal/...
	cd web && $(WEB_BUILD)

build:
	mkdir -p $(BUILD_DIR) $(GOCACHE)
	GOCACHE=$(GOCACHE) go build -o $(BUILD_BIN) ./cmd/server
	cd web && $(WEB_BUILD)

service-install: build
	@$(SERVICE_SCRIPT) install

service-start:
	@$(SERVICE_SCRIPT) start

service-stop:
	@$(SERVICE_SCRIPT) stop

service-restart:
	@$(SERVICE_SCRIPT) restart

service-status:
	@$(SERVICE_SCRIPT) status

service-logs:
	@$(SERVICE_SCRIPT) logs

service-uninstall:
	@$(SERVICE_SCRIPT) uninstall

proxy-status:
	@curl -sS "$(API_BASE)/api/proxy/status"; echo

proxy-start:
	@curl -sS -X POST "$(API_BASE)/api/proxy/start"; echo

proxy-stop:
	@curl -sS -X POST "$(API_BASE)/api/proxy/stop"; echo

proxy-restart:
	@curl -sS -X POST "$(API_BASE)/api/proxy/restart"; echo
