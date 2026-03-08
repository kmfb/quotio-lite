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
- True service mode: Go serves the built frontend without a Vite dev server
- macOS `launchd` install/start/stop/restart/status/logs/uninstall workflow

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
- Supports CLI-based release update (`--check`, `--update --latest|--version`).

### Official Download Sources
- Releases page: [router-for-me/CLIProxyAPI Releases](https://github.com/router-for-me/CLIProxyAPI/releases)
- Recommended: use `make cliproxy-check` to resolve the current latest tag dynamically.

Common asset naming:
- macOS Apple Silicon: `CLIProxyAPI_<version>_darwin_arm64.tar.gz`
- macOS Intel: `CLIProxyAPI_<version>_darwin_amd64.tar.gz`
- Linux amd64: `CLIProxyAPI_<version>_linux_amd64.tar.gz`
- Linux arm64: `CLIProxyAPI_<version>_linux_arm64.tar.gz`
- Windows amd64: `CLIProxyAPI_<version>_windows_amd64.zip`

### CLI update workflow (no UI/productization)

```bash
# Check current binary + latest GitHub release tag
make cliproxy-check

# Update to latest release
make cliproxy-update

# Update to a specific release tag
make cliproxy-update VERSION=v6.8.47
```

The updater verifies SHA256 from release `checksums.txt` when available,
installs to a versioned binary path, then atomically switches the active target.

## Run

### Dev mode

```bash
cd /path/to/quotio-lite
make dev
```

- Backend: `http://127.0.0.1:18417`
- Frontend: `http://127.0.0.1:5173`
- Vite proxies `/api` to the Go backend

Use this for active development only. It intentionally runs two processes.

### Service mode

Service mode is a single Go process that serves:
- API routes from `/api/*`
- built frontend assets from `/`

Build and install the launchd bundle:

```bash
make service-install
```

Manage the macOS user service:

```bash
make service-start
make service-stop
make service-restart
make service-status
make service-logs
make service-uninstall
```

Installed service bundle paths:
- Binary: `~/.quotio-lite/service/bin/quotio-lite`
- Frontend bundle: `~/.quotio-lite/service/web`
- LaunchAgent: `~/Library/LaunchAgents/io.github.kmfb.quotio-lite.plist`

### Safety notes

- `make dev` and `make service-*` are separate workflows; do not blindly manage one while the other is using port `18417`.
- The launchd helpers only manage the `io.github.kmfb.quotio-lite` job and do **not** kill arbitrary processes.
- `make service-start` refuses to start if port `18417` is already occupied by another process, to avoid disrupting an existing dev instance.
- The service workflow does not manage Vite on port `5173`; if you already have a dev frontend there, leave it alone.

Proxy control shortcuts (backend must be running):

```bash
make proxy-status
make proxy-start
make proxy-stop
make proxy-restart
```

## Environment variables
- `QUOTIO_LITE_MODE` (`dev` by default, `service` for single-process mode)
- `QUOTIO_LITE_HOST` (default `127.0.0.1`)
- `QUOTIO_LITE_PORT` (default `18417`)
- `QUOTIO_LITE_AUTH_DIR` (default `~/.cli-proxy-api`)
- `QUOTIO_LITE_CLIPROXY_PATH` (default `~/.quotio-lite/bin/CLIProxyAPI`)
- `QUOTIO_LITE_FRONTEND_DIST_DIR` (default `./web/dist` relative to the current working directory)
- `QUOTIO_LITE_CLIPROXY_VERSION_DIR` (default `~/.quotio-lite/bin/versions`)
- `QUOTIO_LITE_CLIPROXY_GITHUB_REPO` (default `router-for-me/CLIProxyAPI`)
- `QUOTIO_LITE_PROBE_MODEL` (default `gpt-5.1-codex-mini`)
- `QUOTIO_LITE_PROBE_TIMEOUT_SEC` (default `50`)
- `QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL` (optional, auto-download URL override for ensure mode)
- `QUOTIO_LITE_CLIPROXY_SHA256` (optional, SHA256 for override download)

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
