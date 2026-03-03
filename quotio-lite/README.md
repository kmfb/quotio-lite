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

## Run

```bash
cd /Users/tian/Documents/Playground/quotio-lite
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

## API
- `GET /api/meta`
- `GET /api/accounts`
- `GET /api/accounts/{file}`
- `POST /api/accounts/login`
- `DELETE /api/accounts/{file}`
- `POST /api/accounts/{file}/probe`
