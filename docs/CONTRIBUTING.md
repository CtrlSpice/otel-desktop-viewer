# Contributor guide

Bug reports, docs fixes, design work and code are all welcome. Please read our
[Code of Conduct](CODE_OF_CONDUCT.md) before participating.

## Choose a change

- [Report a bug](https://github.com/CtrlSpice/otel-desktop-viewer/issues/new?template=bug_report.yml): include steps to reproduce it, what you expected and what happened.
- [Suggest a feature](https://github.com/CtrlSpice/otel-desktop-viewer/issues/new?template=feature_request.yml): explain the problem you want to solve.
- For a first contribution, small fixes and documentation improvements are good places to start. Open an issue if you would like help choosing one.

For larger product or architectural changes, or changes to the
[component registry](../components.go), discuss the approach and trade-offs in an
issue and wait for maintainer agreement before starting. This includes changes
to storage, APIs, routing, saved state and shared UI behaviour. Ask if you are
unsure about the scope.

AI tools are allowed. Review and understand any output you submit and keep it
within the agreed scope. You remain responsible for the contribution and review
follow-up.

## Run locally

You'll need Git, Make and the following tools:

| Tool | Requirement |
| --- | --- |
| Go | 1.26; see [`go.mod`](../go.mod). |
| Node.js and npm | Node 26; see [`.nvmrc`](../.nvmrc). |
| C compiler | CGO is required. On Windows, use MinGW64 GCC compatible with DuckDB's prebuilt library; CI uses `C:\mingw64\bin`. |

Restricted Go module proxies must allow `go.duckdb.org`, which hosts DuckDB's
prebuilt native libraries.

Clone the repository and install locked frontend dependencies and Playwright Chromium:

```bash
git clone https://github.com/CtrlSpice/otel-desktop-viewer.git
cd otel-desktop-viewer
make install-clean
```

### Start the backend and frontend

Start a backend if one is not already running:

```bash
make run-go
```

In another terminal, start Vite with hot reload:

```bash
make dev-ts
```

Open <http://localhost:3001>. Vite proxies the viewer's API requests to the
backend at `http://localhost:8000`. For a backend on another port:

```bash
VIEWER_PROXY_TARGET=http://localhost:8001 make dev-ts
```

Stop each server with Ctrl-C in its own terminal.

### Load sample telemetry

With the server running:

```bash
make populate-traces populate-logs populate-metrics
```

This posts the fixed requests in `testdata/otlp/small/` to `http://localhost:4318`.
Choose **All** or the dataset's recorded time window in the viewer. Replaying logs
or datapoints appends records. Restart the default in-memory backend before
seeding again if you need repeatable counts.

Set `OTLP_DATASET` to choose another dataset and `OTLP_ENDPOINT` to change the receiver:

```bash
make populate-traces populate-logs populate-metrics \
  OTLP_DATASET=testdata/otlp/checkout OTLP_ENDPOINT=http://localhost:4318
```

See the [dataset guide](../testdata/otlp/README.md) for chart fixtures and request counts.

### Build the standalone viewer

Stop your development backend, then:

```bash
make build
./otel-desktop-viewer
```

The viewer opens <http://localhost:8000>. `make build` builds the frontend before
compiling Go; plain `go build` uses the committed embedded frontend.

Builds default to `EVALS=0`. `EVALS=1` includes the experimental evaluation-results
workspace. Match the backend and frontend settings during development and tests.

## Make your change

Keep changes focused and follow nearby code and tests. Read the
[architecture guide](ARCHITECTURE.md) before changing component ownership or
shared behaviour. It covers the project layout, data flow and UI conventions.

Preserve received types, exact values and complete Copy/Download output.
Keep installation and quick-start instructions in the README, with detailed
references in `docs/` and links from [the documentation index](README.md).
Document functionality on `main`; keep plans, research, drafts and handoffs
untracked and out of public documentation links.

### Go

- Run Go commands from the repository root. Use `any` for empty interface types and format with `gofmt`.
- Access DuckDB through `Store.WithConn`, `Store.WithDBRead` or `Store.WithDBWrite` to preserve locking and connection ownership.
- `main.go` and `components.go` are maintained by hand. Keep module versions aligned with `go.mod` and run `TestComponentModuleVersionsMatchGoMod`.
- Preserve API contracts, validate server inputs and handle expected store errors explicitly.
- When changing [trace, log or metric search results](ARCHITECTURE.md#shared-searches-and-output-formats), update [CLI consumers](../root_command.go), the [OpenRPC contract](../desktopexporter/internal/server/openrpc.go), [agent guide](../skills/otel-desktop-viewer/SKILL.md) and frontend. Check CLI changes against the real server.

For persisted schema changes, follow the [migration guide](schema-migration.md):
bump the version, add an immutable SQL migration, update current DDL and its
`_order` manifests, and test populated upgrades, fresh-schema equivalence and
rollback. Schema 27 is the baseline; both builds share the schema and migration chain.

### Frontend

The Svelte 5/TypeScript app lives in `desktopexporter/internal/frontend/`.
Reuse shared components, workspace panes, disclosure controls and saved
preferences. Follow the [workspace](ARCHITECTURE.md#persistent-workspace-and-sections)
and [theme conventions](ARCHITECTURE.md#themes-and-data-presentation), including
shared typography and colour tokens. Adjust layouts rather than shrinking text.

### Regenerate affected files

- For search grammar changes, run `npm run generate:grammar` in the frontend directory and commit the generated parser.
- Generate theme and graph colours from the pinned source with `node scripts/generate-madoromi.mjs` in that directory.
- For dependency changes, use `npm install` in the frontend directory and include the manifest and lockfile updates.
- After frontend source, dependency or build-configuration changes, build the public assets with `make build-ts EVALS=0` from the root and commit `desktopexporter/internal/server/static/`.

Resolve source and dependency conflicts before regenerating bundles; do not
hand-merge generated assets.

## Check your change

Behaviour and dependency changes need focused automated tests. Test pure logic
directly and cover meaningful edge cases and state transitions. Add integration
tests for state, routing and storage changes, and handler/store tests for API
changes. For a bug fix, show that the test fails before the fix where practical.
If the code lacks a useful test boundary, discuss and add one before opening the PR.

For UI changes, check the running app's:

- Layouts, themes and zoom levels.
- Keyboard access, focus, screen-reader names, reduced motion and visible alternatives to audio.
- Loading, empty and error states.

Keep screenshot and recording evidence local. For dependency updates, manually
check the affected UI or backend behaviour too.

Before marking the PR ready for review, run the full suite from the repository root:

```bash
make test
```

This checks formatting, lint, types, embedded assets, Go/Vitest tests and browser
accessibility. The individual targets are:

| Check | Command |
| --- | --- |
| Go tests | `make test-go` |
| Go formatting | `make format-go-check` |
| Frontend formatting | `make format-ts-check` |
| Frontend lint | `make lint-ts` |
| Frontend types | `make validate-ts` |
| Playwright types | `make validate-playwright` |
| Frontend unit tests | `make test-ts` |
| Browser accessibility | `make test-a11y` |
| Embedded assets | `make build-ts-check` |

The asset check compares public assets with committed files; `EVALS=1` checks compilation only.

Optional: check a public executable with fresh storage and its embedded UI:

```bash
go build -o otel-desktop-viewer
node desktopexporter/internal/frontend/scripts/smoke-binary.mjs ./otel-desktop-viewer
```

CI runs this smoke test, checks parser generation and guarded store access, and
builds/tests Go on Linux, macOS and Windows.
Run `git diff --check` before committing.

## Open a pull request

Use a draft PR while the work or checks are still in progress. Write the description
as short, standalone bullet points explaining what changed and why. Include
change-specific verification where useful. Use `Fixes #…` or `Closes #…` when the
change resolves an issue and matches the agreed scope.

Review may ask for a different approach, more tests or a smaller scope. Maintainers
may defer or close work outside the project's direction or review capacity, or
close inactive PRs.

## License

By contributing, you agree that your contributions will be licensed under the project's [Apache 2.0 license](../LICENSE).
