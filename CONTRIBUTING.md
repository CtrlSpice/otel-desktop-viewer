# Contributing

Thanks for stopping by. Bug reports, documentation, interface polish, backend work, and thoughtful experiments are all welcome.

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) first. We want contributing to feel as approachable as the application.

## Contribution principles

- Coordinate substantial product and architecture decisions with a maintainer before implementation.
- Understand and take responsibility for every submitted change.
- Include focused automated tests with every behavioral change.
- Preserve the repository's architecture, interaction patterns, accessibility, and generated-file conventions.
- Keep pull requests small enough to review and verify with confidence.

### AI tools and coding agents

AI tools and coding agents are welcome when a contributor supervises them closely and keeps them within the agreed scope. Agent-assisted work is welcome when it is well-behaved: the contributor reviews and understands every change, brings product and architecture decisions to maintainers, controls generated output, and includes focused tests for every behavioral change. Focused tests are required for agent-assisted contributions.

The contributor owns the pull request, review discussion, corrections, and follow-through regardless of which tools produced the first draft.

## Choosing work

- **Report bugs** using the [bug report template](.github/ISSUE_TEMPLATE/bug_report.yml). Reproduction steps, logs, and screenshots are especially helpful.
- **Suggest features** using the [feature request template](.github/ISSUE_TEMPLATE/feature_request.yml).
- **Improve documentation** in the README, [ARCHITECTURE.md](ARCHITECTURE.md), and comments around non-obvious behavior.
- **Send a pull request** after following the coordination guidance below.

If you are looking for a first contribution, open an issue and say hello. Small bug fixes, documentation improvements, and narrowly scoped polish are good places to start.

### Before substantial work

Maintainers use open issues to understand problems and gather context. Agreement on an implementation approach is a separate step. Before starting a substantial user-facing, architectural, data-model, persistence, or cross-cutting change:

- Comment on the issue with the approach you intend to take.
- Wait for maintainer agreement on the direction.
- Surface important product and architectural trade-offs in the discussion.
- Use `Fixes #...` or `Closes #...` after the implementation matches the agreed scope.

Ask when the boundary is unclear. Maintainers may defer or close substantial pull requests that began without alignment, including completed implementations. A completed implementation carries no guarantee of merge.

## Prerequisites

| Tool                                 | Version / notes                                                                        |
| ------------------------------------ | -------------------------------------------------------------------------------------- |
| [Go](https://go.dev/)                | 1.26 (see `go.mod`)                                                                    |
| [Node.js](https://nodejs.org/) + npm | Node 26 (see `.nvmrc`)                                                                 |
| CGO                                  | Required; DuckDB bindings need a compatible C toolchain                                |
| Windows                              | MinGW64 GCC compatible with DuckDB's prebuilt static library; CI pins `C:\mingw64\bin` |

Clone the repo:

```bash
git clone https://github.com/CtrlSpice/otel-desktop-viewer.git
cd otel-desktop-viewer
make install
```

This installs frontend packages and the Chromium build used by Playwright accessibility tests.

## Development workflow

The app is a custom OpenTelemetry Collector binary plus a Svelte 5 UI. See [ARCHITECTURE.md](ARCHITECTURE.md) for the full picture.

### Quick start with sample data

```bash
make dev-go
```

This starts the Go server on `:8000`, seeds traces, logs, and metrics, and opens the embedded UI.

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

`make build-ts` copies the frontend into `desktopexporter/internal/server/static/` for embedding. Run it before testing frontend changes through the standalone binary.

### Seed scripts

With the server running:

```bash
make populate-traces
make populate-logs
make populate-metrics
```

Seeding is driven by `scripts/seed.pl` (`--traces`, `--logs`, `--metrics`, `--all`). Run `populate-traces` before `populate-logs` so linked log records can resolve their traces. Override the endpoint with `OTLP_ENDPOINT=http://host:4318` when needed.

### Stop dev servers

```bash
make stop
```

## Architecture and project layout

Read [ARCHITECTURE.md](ARCHITECTURE.md) before changing component ownership, storage, ingestion, JSON-RPC contracts, routing, or persisted state.

```
main.go, components.go                # Collector entry and OCB-generated registry
desktopexporter/
  exporter.go                         # Signal exporters; writes through the shared store
  duckdbextension/                    # Store, HTTP server, and retention ownership
  internal/server/                    # HTTP and JSON-RPC
  internal/store/                     # DuckDB schema, ingestion, and queries
  internal/frontend/                  # Svelte 5 application
scripts/                              # OTLP seed data for local development
docs/                                 # All design notes and documentation assets
```

The root Go module builds the collector binary. The `desktop` exporter writes telemetry through the store owned by the `duckdb` extension. The frontend communicates with the server through JSON-RPC and builds into committed embedded assets.

## Change conventions

Keep repository documentation under `docs/`. Working notes that are worth
preserving belong there too; do not create a parallel `notes/` directory.

### Go

- Run Go commands from the repository root so root-package tests remain included.
- Use the modern `any` alias for empty interface types.
- Format with `gofmt` and verify with `make format-go-check`.
- Access DuckDB through `Store.WithConn`, `Store.WithDBRead`, or `Store.WithDBWrite`. Production code must preserve the store's locking and connection ownership.
- Treat `main.go` and `components.go` as OCB-generated collector wiring. Coordinate component-registry changes with a maintainer and keep recorded module versions synchronized.
- Run `make test-go` or `go test ./...` from the repository root.

Discuss DuckDB schema, retention, ingestion, and store-lifecycle changes before implementation. Schema objects live under `desktopexporter/internal/store/queries/ddl/`; update the applicable `_order` manifest and schema-version expectations with every schema change.

JSON-RPC changes should preserve deliberate wire contracts, validate input at the server boundary, map expected store errors explicitly, and include handler plus store-level tests.

### Frontend

- Svelte 5 + TypeScript + Vite + Tailwind/DaisyUI under `desktopexporter/internal/frontend/`.
- Format: `make format-ts`
- Typecheck: `make validate-ts`
- Unit tests: `make test-ts`
- Accessibility tests: `make test-a11y`

Match nearby components, contexts, route helpers, and test fixtures before introducing new abstractions. Discuss changes to routing, persisted state, navigation behavior, or shared interaction patterns as product decisions before implementation.

### Dependencies and generated files

The repository commits several generated outputs because release builds consume them directly:

- Search parser files under `src/components/shared/Search/codemirror/` are generated from `query.grammar`. Run `npm run generate:grammar` from `desktopexporter/internal/frontend/` and commit the result.
- Production frontend assets live under `desktopexporter/internal/server/static/`. Frontend source, dependency, lockfile, and build-configuration changes require `make build-ts` and the resulting committed assets.
- Resolve source and dependency conflicts first, then regenerate hashed frontend assets from the combined tree. Hand-merging generated bundles produces unreliable output.
- Dependency updates should include the manifest, lockfile, generated bundle where applicable, focused regression coverage, and manual checks for affected product surfaces.

From `desktopexporter/internal/frontend/`, use `npm ci` to verify the committed lockfile and `npm install` when intentionally changing dependency resolution.

## Tests and product quality

Every behavioral change requires focused automated tests at the nearest useful boundary. Tests should:

- Exercise the changed code and intended contract directly.
- Demonstrate the regression by failing against the previous behavior when practical.
- Cover pure parsing, comparison, and transformation logic directly.
- Add integration coverage for behavior involving state, routing, persistence, SQL, JSON-RPC, or subsystem boundaries.
- Cover meaningful edge cases and state transitions alongside the main path.
- Follow the structure and conventions of nearby tests.

Run the broad suite in addition to the focused tests. Generated assets and incidental execution coverage provide build evidence; targeted tests establish the behavior under review. When the current harness lacks a useful test seam, include that seam in the change and discuss the approach with a maintainer before submitting the pull request.

For interface work, verify both visual presentation and assistive access in the running application. Preserve the established visual language and include screenshots or short recordings with a text description for visible changes. Check the relevant responsive layouts, themes, zoom behavior, keyboard operation, focus management, screen-reader names, visible alternatives for audio feedback, reduced-motion behavior, and loading, empty, and error states.

Automated component and Playwright/axe tests should target the affected interaction. Maintainers also review usability, visual quality, and accessibility as product behavior.

## Verification

Run focused tests while developing. Before marking a pull request ready for review, run:

```bash
make test
```

`make test` verifies Go and frontend formatting, Svelte and Playwright types, the embedded frontend build, Go tests, Vitest tests, and browser accessibility tests.

Useful focused targets include:

| Area                        | Command                |
| --------------------------- | ---------------------- |
| Go tests                    | `make test-go`         |
| Go formatting               | `make format-go-check` |
| Frontend formatting         | `make format-ts-check` |
| Frontend types              | `make validate-ts`     |
| Frontend unit tests         | `make test-ts`         |
| Browser accessibility       | `make test-a11y`       |
| Embedded frontend freshness | `make build-ts-check`  |

Run `git diff --check` before committing. CI regenerates the search parser and checks for a clean diff, enforces guarded store access, and builds plus tests the Go application on Ubuntu, macOS, and Windows.

## Opening a pull request

Use a draft pull request while implementation or verification is still in progress. Complete the pull request template before requesting review:

- Explain the problem and why the change belongs in the project.
- Link the issue discussion where a maintainer agreed to a substantial approach.
- Describe the implementation and important trade-offs.
- Name the focused tests and the behavior each test establishes.
- Include exact verification commands and results.
- Include visual and accessibility evidence for interface changes.
- Include regenerated parser or embedded frontend assets where applicable.

Keep each pull request focused and independently reviewable. Respond to review with the same care used for the implementation, and regenerate derived files after resolving source conflicts.

## Review and merge

Maintainers evaluate architectural fit, product behavior, focused tests, visual and assistive quality, maintainability, and repository consistency. Review may ask for a different design, additional tests, a smaller scope, or a fresh implementation based on an agreed approach.

Maintainers decide what enters the project and may defer or close work that falls outside the current direction or review capacity. Stale pull requests may close after a reasonable period without activity.

## License

By contributing, you agree that your contributions will be licensed under the project's [Apache 2.0 license](LICENSE).
