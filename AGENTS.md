# Repository documentation

- Use plain bullet points as the entire PR description.
- Use concise, standalone bullet points explaining what changed and why.
- Write in plain language that makes sense six months later without the conversation.
- Keep each bullet focused on concrete behaviour.
- Preserve wording the user supplies or corrects.
- Include verification when it adds new, change-specific evidence.
- Apply these rules automatically whenever creating or updating a PR.

- Do not use em dashes in chat or documentation. Use a colon, comma or separate sentence.

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
- Keep installation and quick-start instructions in `README.md`. Keep detailed
  reference and configuration guides in `docs/`, linked from `docs/README.md`.
  Write standalone Markdown guides with explicit defaults and complete examples.
- Use descriptive noun titles for standalone documents, such as "Docker guide".
  Use action headings for procedural sections within documents.
- Reference guides use descriptive lowercase, hyphen-separated filenames. Match
  each guide's page title and link label in docs/README.md and the root README.
  Keep README.md, CONTRIBUTING.md and CODE_OF_CONDUCT.md under their standard
  GitHub-recognised filenames.
- Keep Lulu artwork and its credit page in assets/lulu/. Keep only images used
  by the README in docs/screenshots/. Review screenshots belong in local-only
  notes/, not docs/evidence/ or the published screenshot directory.
- Keep guides for features not yet in production, including experimental
  features and Evals ingestion, in local-only `notes/`. Move them into `docs/`
  when those features ship in production. This keeps public documentation focused
  on production functionality.
