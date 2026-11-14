# Contributor guide

Bug reports, docs fixes, design work and code are welcome. Please follow our
[Code of Conduct](CODE_OF_CONDUCT.md).

## Choose a change

[Open an issue](https://github.com/CtrlSpice/otel-desktop-viewer/issues/new/choose)
to report a bug, suggest a feature or ask for help choosing a first contribution.

Before substantial product or architectural work, discuss the approach in an
issue and wait for maintainer agreement.

AI tools are allowed. Review and understand their output, keep it within the
agreed scope, and take responsibility for the contribution and review follow-up.

## Run locally

You need Git, Make, [Go 1.26](../go.mod), [Node 26](../.nvmrc) with npm,
and a C compiler for DuckDB. Windows requires
[MinGW64](../README.md#1-install-the-viewer).

```bash
git clone https://github.com/CtrlSpice/otel-desktop-viewer.git
cd otel-desktop-viewer
make install-clean
```

Start the backend:

```bash
make run-go
```

In another terminal, start the frontend with hot reload:

```bash
make dev-ts
```

Open <http://localhost:3001>. The backend runs on port 8000 and keeps data in
memory. To load sample telemetry:

```bash
make populate-traces populate-logs populate-metrics
```

Select **All** in the viewer's time range to see it. See the
[dataset guide](../testdata/otlp/README.md) for other samples.

## Check your change

Keep changes focused and follow nearby code. Behaviour and dependency changes
need focused automated tests. For bug fixes, show that the test fails before
the fix where practical. Check UI changes in the running app for accessibility
and visual regressions.

Read [Architecture](architecture.md) for code and UI conventions, and follow
[Database upgrades](database-upgrades.md) for schema changes.

After frontend source, dependency or build-configuration changes, regenerate
the public assets from the repository root and commit the updated
`desktopexporter/internal/server/static/` files:

```bash
make build-ts EVALS=0
```

Before marking the PR ready, run from the repository root:

```bash
make test
git diff --check
```

The [Makefile](../Makefile) lists individual build and test commands.

## Open a pull request

Use a draft while work or checks are incomplete. Describe what changed and why
in short bullet points, with relevant test results. Add `Fixes #…` or `Closes #…`
when the change resolves an issue and matches the agreed scope.

By contributing, you agree to license your work under the project's
[Apache 2.0 license](../LICENSE).
