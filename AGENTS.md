# Repository documentation

- Features require build-time selection; runtime-only feature flags are not
  supported. Public builds use `EVALS=0`; `EVALS=1` selects Evals in Go and the
  frontend. Preserve shared viewer/database functionality in both builds.

- Schema 27 (0.7.0) is the migration baseline. Persisted schema changes require
  a version bump, an immutable SQL migration, matching fresh DDL, and populated
  upgrade/rollback tests. Both builds use the same migration chain.

- `docs/` is live documentation for functionality on `main`. Describe current
  behavior, supported contracts, and usage. Do not put proposals, roadmaps,
  implementation plans, or private handoffs there.
- `notes/` is private, local working material. Put plans, research, drafts, and
  handoffs there. Do not commit, force-add, or publish its contents, or link to
  private notes from public documentation.
- When planned functionality lands on `main`, document its implemented behavior
  in `docs/`; do not publish the working notes as product documentation.
