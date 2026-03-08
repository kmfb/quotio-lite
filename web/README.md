# quotio-lite web

Frontend for Quotio-Lite.

## Commands

```bash
corepack pnpm install
corepack pnpm dev
corepack pnpm build
corepack pnpm lint
```

By default it uses same-origin `/api` requests.
In `make dev`, Vite proxies `/api` to `http://127.0.0.1:18417`.
Override with `VITE_API_BASE_URL` if needed.
