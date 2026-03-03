# quotio-lite

Local Codex account control panel with:
- account onboarding (`CLIProxyAPI -codex-login`)
- credential cleanup/delete
- account probe
- per-account quota visibility (5h / weekly)

Project source is in:
- [`quotio-lite/`](./quotio-lite)

Quick start:

```bash
cd quotio-lite
make dev
```

Then open:
- backend: `http://127.0.0.1:18417`
- frontend: `http://127.0.0.1:5173`
