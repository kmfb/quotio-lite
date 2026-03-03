# quotio-lite

Local Codex account control panel for Quotio-style workflows.

## Stack
- Backend: Go
- Frontend: Vite + React + TanStack Query/Router/Table
- UI: Radix Themes
- Package manager: pnpm

## Features (v1)
- List Codex credential files from `~/.cli-proxy-api`
- Add account via `CLIProxyAPI -codex-login`
- Delete account by exact filename
- Manual probe with model `gpt-5.1-codex-mini`
- Fine-grained status classification
- Account detail page
- Account list polling every 15s (background tab supported)
- Per-account 5h/weekly usage from `https://chatgpt.com/backend-api/wham/usage`

## Prerequisites
- Go `1.25+`
- Node.js `20+`
- `pnpm` (or use `corepack pnpm`)
- Codex credential directory `~/.cli-proxy-api`
- `CLIProxyAPI` binary (typically installed by Quotio)

Notes:
- `CLIProxyAPI` is required for `POST /api/accounts/login` and `POST /api/accounts/{file}/probe`.
- Listing/deleting accounts and reading quota can still work without `CLIProxyAPI`, as long as credential files exist.
- We intentionally **do not vendor** `CLIProxyAPI` in this repository:
  - closed-source/third-party binary lifecycle
  - frequent upstream updates and compatibility risk
  - larger repo size and supply-chain/security concerns

## Bootstrap Runtime Dependency
You can auto-resolve `CLIProxyAPI` before running:

```bash
make bootstrap
```

Behavior:
- Reuses an existing local `CLIProxyAPI` if found.
- If not found, it can auto-download when `QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL` is set.
- Optional integrity check via `QUOTIO_LITE_CLIPROXY_SHA256`.

## Run

```bash
cd /path/to/quotio-lite
make dev
```

- Backend: `http://127.0.0.1:18417`
- Frontend: `http://127.0.0.1:5173`

## Environment variables
- `QUOTIO_LITE_HOST` (default `127.0.0.1`)
- `QUOTIO_LITE_PORT` (default `18417`)
- `QUOTIO_LITE_AUTH_DIR` (default `~/.cli-proxy-api`)
- `QUOTIO_LITE_CLIPROXY_PATH` (default Quotio app CLIProxyAPI path)
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
