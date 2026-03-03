# quotio-lite

Local Codex account control panel for Quotio-style workflows.

## Stack
- Backend: Go
- Frontend: Vite + React + TanStack Query/Router/Table
- UI: Radix Themes
- Package manager: pnpm

## Features (v1.1)
- List Codex credential files from `~/.cli-proxy-api`
- Add account via `CLIProxyAPI -codex-login`
- Delete account by exact filename
- Manual probe with model `gpt-5.1-codex-mini`
- Fine-grained status classification
- Account detail page
- Account/detail polling every 60-120s with randomized intervals (foreground tab)
- Per-account 5h/weekly usage from `https://chatgpt.com/backend-api/wham/usage`
- Managed `CLIProxyAPI` runtime (start/stop/restart) on `127.0.0.1:8317`
- Copy-ready endpoint + API key for other OpenAI-compatible apps
- One-click API key rotation (single persistent key strategy)
- Conservative usage caching + jitter + backoff to reduce rate-limit risk

## Prerequisites
- Go `1.25+`
- Node.js `20+`
- `pnpm` (or use `corepack pnpm`)
- Codex credential directory `~/.cli-proxy-api`
- `CLIProxyAPI` binary

Notes:
- `CLIProxyAPI` is required for `POST /api/accounts/login` and `POST /api/accounts/{file}/probe`.
- Listing/deleting accounts and reading quota can still work without `CLIProxyAPI`, as long as credential files exist.
- We intentionally **do not vendor** `CLIProxyAPI` in this repository:
  - closed-source/third-party binary lifecycle
  - frequent upstream updates and compatibility risk
  - larger repo size and supply-chain/security concerns

## Bootstrap Runtime Dependency
Default binary path is:

```bash
~/.quotio-lite/bin/CLIProxyAPI
```

You can auto-resolve `CLIProxyAPI` before running:

```bash
make bootstrap
```

Behavior:
- Reuses an existing local `CLIProxyAPI` if found.
- If not found, it can auto-download when `QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL` is set.
- Optional integrity check via `QUOTIO_LITE_CLIPROXY_SHA256`.
- Supports direct binaries and archives (`.tar.gz`, `.zip`) and auto-extracts `CLIProxyAPI`.
- Keeps compatibility candidates from old Quotio install paths.

### Official Download Sources
- Releases page: [router-for-me/CLIProxyAPI Releases](https://github.com/router-for-me/CLIProxyAPI/releases)
- Latest tag (checked on 2026-03-03): [`v6.8.40`](https://github.com/router-for-me/CLIProxyAPI/releases/tag/v6.8.40)

Common assets:
- macOS Apple Silicon: `CLIProxyAPI_6.8.40_darwin_arm64.tar.gz`
- macOS Intel: `CLIProxyAPI_6.8.40_darwin_amd64.tar.gz`
- Linux amd64: `CLIProxyAPI_6.8.40_linux_amd64.tar.gz`
- Linux arm64: `CLIProxyAPI_6.8.40_linux_arm64.tar.gz`
- Windows amd64: `CLIProxyAPI_6.8.40_windows_amd64.zip`

Example (macOS Apple Silicon):

```bash
export QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL="https://github.com/router-for-me/CLIProxyAPI/releases/download/v6.8.40/CLIProxyAPI_6.8.40_darwin_arm64.tar.gz"
export QUOTIO_LITE_CLIPROXY_SHA256="<paste-from-checksums.txt>"
make bootstrap
```

## Run

```bash
cd /path/to/quotio-lite
make dev
```

- Backend: `http://127.0.0.1:18417`
- Frontend: `http://127.0.0.1:5173`

Proxy control shortcuts (backend must be running):

```bash
make proxy-status
make proxy-start
make proxy-stop
make proxy-restart
```

## Environment variables
- `QUOTIO_LITE_HOST` (default `127.0.0.1`)
- `QUOTIO_LITE_PORT` (default `18417`)
- `QUOTIO_LITE_AUTH_DIR` (default `~/.cli-proxy-api`)
- `QUOTIO_LITE_CLIPROXY_PATH` (default `~/.quotio-lite/bin/CLIProxyAPI`)
- `QUOTIO_LITE_PROBE_MODEL` (default `gpt-5.1-codex-mini`)
- `QUOTIO_LITE_PROBE_TIMEOUT_SEC` (default `50`)
- `QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL` (optional, auto-download URL for CLIProxyAPI)
- `QUOTIO_LITE_CLIPROXY_SHA256` (optional, sha256 check for downloaded binary)

## API
- `GET /api/meta`
- `GET /api/accounts`
- `GET /api/accounts/{file}`
- `POST /api/accounts/login`
- `DELETE /api/accounts/{file}`
- `POST /api/accounts/{file}/probe`
- `GET /api/proxy/status`
- `POST /api/proxy/start`
- `POST /api/proxy/stop`
- `POST /api/proxy/restart`
- `GET /api/proxy/credentials`
- `POST /api/proxy/api-key/rotate`
