# Contributor guide

Hi, welcome. I'm happy to have you.

Bug reports, docs fixes, design work and code are welcome. Please follow our
[Code of Conduct](CODE_OF_CONDUCT.md).

If you're new to open source and want to help but aren't sure where to start,
please [poke me](mailto:amelia.ardath@gmail.com), publicly or privately, however
you're comfortable. I'm happy to help you find your way around the project and
choose a first contribution.

## Choose a change

[Open an issue](https://github.com/CtrlSpice/otel-desktop-viewer/issues/new/choose)
to report a bug, suggest a feature or ask for help choosing a first contribution.

Before substantial product or architectural work, discuss the approach in an
issue and wait for maintainer agreement.

You can use AI tools, but reviewers should not be able to tell. Review and
understand their output, keep it within the agreed scope, and take responsibility
for the contribution and review follow-up.

## Run locally

You need:

- [<img src="https://cdn.simpleicons.org/git/F05032" width="16" height="16" alt="" style="display: inline-block; vertical-align: middle; margin: 0;"> Git](https://git-scm.com/downloads/)
- [<img src="https://cdn.simpleicons.org/gnu/A42E2B" width="16" height="16" alt="" style="display: inline-block; vertical-align: middle; margin: 0;"> Make](https://www.gnu.org/software/make/)
- [<img src="https://cdn.simpleicons.org/go/00ADD8" width="16" height="16" alt="" style="display: inline-block; vertical-align: middle; margin: 0;"> Go 1.26](https://go.dev/doc/install)
- [<img src="https://cdn.simpleicons.org/nodedotjs/5FA04E" width="16" height="16" alt="" style="display: inline-block; vertical-align: middle; margin: 0;"> Node 26](https://nodejs.org/en/download) with npm
- [<img src="https://cdn.simpleicons.org/c/A8B9CC" width="16" height="16" alt="" style="display: inline-block; vertical-align: middle; margin: 0;"> A C compiler](../README.md#1-install-the-viewer) for DuckDB. Windows requires MinGW64.

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

Open <http://localhost:3001>. The backend runs on port 8000 and stores data in
managed sessions. To load sample telemetry:

```bash
make populate
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
