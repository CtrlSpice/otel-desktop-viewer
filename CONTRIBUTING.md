# Contributing

Thanks for stopping by. Bug reports, docs fixes, UI polish, backend work — all welcome.

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) first. We want this project to feel as approachable as the app tries to be.

## Ways to help

- **Report bugs** — use the [bug report template](.github/ISSUE_TEMPLATE/bug_report.yml). Steps to reproduce and screenshots go a long way.
- **Suggest features** — use the [feature request template](.github/ISSUE_TEMPLATE/feature_request.yml).
- **Improve docs** — README, [ARCHITECTURE.md](ARCHITECTURE.md), comments where behavior is non-obvious.
- **Send a pull request** — see below.

Not sure where to start? Open an issue and say hello. Small fixes are great first contributions.

## Prerequisites

| Tool | Version / notes |
|------|-----------------|
| [Go](https://go.dev/) | 1.26 (see `go.mod`) |
| [Node.js](https://nodejs.org/) + npm | For the Svelte frontend |
| CGO | Required — DuckDB bindings need a C toolchain |
| Windows | [MSYS2 UCRT64](https://www.msys2.org/) + GCC (see [README](README.md#getting-started)) |

Clone the repo:

```bash
git clone https://github.com/CtrlSpice/otel-desktop-viewer.git
cd otel-desktop-viewer
make install
```

## Development workflow

The app is a custom OpenTelemetry Collector binary plus a Svelte 5 UI. See [ARCHITECTURE.md](ARCHITECTURE.md) for the full picture.

### Quick start with sample data

```bash
make dev-go
```

Starts the Go server on `:8000`, seeds traces/logs/metrics, and opens the embedded UI.

### Frontend development (recommended for UI work)

Use two terminals:

```bash
# Terminal 1 — backend + sample telemetry
make dev-go

# Terminal 2 — Vite with hot reload
make dev-ts
```

Open **http://localhost:3001**. Vite proxies `/rpc` to the Go server on `:8000`.

### Production-like run

```bash
make build
./otel-desktop-viewer
```

`make build-ts` copies the frontend into `desktopexporter/internal/server/static/` for embedding. If you change frontend code and want the standalone binary to pick it up, run `make build-ts` (or `make build`) before testing `./otel-desktop-viewer`.

### Seed scripts

With the server running:

```bash
make populate-traces
make populate-logs
make populate-metrics
```

Seeding is driven by `scripts/seed.pl` (`--traces`, `--logs`, `--metrics`, `--all`). Run `populate-traces` before `populate-logs` so a handful of log records link to traces that actually exist (the UI's log → trace deep link). Override the endpoint with `OTLP_ENDPOINT=http://host:4318` if needed.

### Stop dev servers

```bash
make stop
```

## Project layout (short version)

```
main.go, components.go     # Collector entry (OCB-generated — edit carefully)
desktopexporter/           # Custom desktop exporter
  internal/server/         # HTTP + JSON-RPC
  internal/store/          # DuckDB ingest and search
  internal/frontend/         # Svelte 5 UI
scripts/                   # OTLP seed data for local dev
docs/                      # README screenshots and assets
```

**Do not hand-edit** OCB-generated collector wiring in `main.go` / `components.go` unless you know you need to — prefer changes in `desktopexporter/` and regenerate via the collector builder when updating components.

## Making changes

### Go

- Code lives mainly under `desktopexporter/`.
- Use `any` instead of `interface{}`.
- Run tests: `make test-go` or `cd desktopexporter && go test ./...`

Store and JSON-RPC handlers are where most backend logic lives. DuckDB schema changes need careful thought — see ARCHITECTURE.md.

### Frontend

- Svelte 5 + TypeScript + Vite + Tailwind/DaisyUI under `desktopexporter/internal/frontend/`.
- Format: `make format-ts`
- Typecheck: `make validate-ts`
- Tests: `cd desktopexporter/internal/frontend && npm test`

Match existing patterns in nearby components before introducing new abstractions.

### Static assets in git

Production builds embed frontend output under `desktopexporter/internal/server/static/`. If your PR changes the UI, include the updated static assets from `make build-ts` so CI and release builds match.

## Before you open a PR

```bash
make test
```

That runs Go tests and frontend typechecking — the same bar we expect locally before review.

CI also builds on **Ubuntu, macOS, and Windows** on every pull request. Windows needs CGO + MSYS2, same as local dev.

In your PR description:

- Say **what** changed and **why**
- Note how you tested it (commands, screenshots for UI changes)
- Link related issues if there are any

Keep PRs focused when you can — easier to review, easier to merge.

## License

By contributing, you agree that your contributions will be licensed under the project's [Apache 2.0 license](LICENSE).
