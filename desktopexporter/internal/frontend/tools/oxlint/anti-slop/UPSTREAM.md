# Upstream provenance

- Source: https://github.com/dmmulroy/anti-slop
- Commit: `95a56e5d24fb3d849673c2d51eb0908b8bd2d33b`
- Installed paths: `index.ts`, `rules/`, `shared/`, and `effect/`
- License: MIT; see `LICENSE`
- Local changes: none

This directory contains the production assets copied by upstream's installer.
Upstream's RuleTester suites are not vendored.

The optional Effect plugin is present in `effect/` because it is part of the
upstream source snapshot, but it is not enabled because this project does not
depend directly on Effect.
