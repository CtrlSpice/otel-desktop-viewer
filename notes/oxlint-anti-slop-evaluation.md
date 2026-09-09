# Oxlint and anti-slop evaluation

Evaluated on 2026-09-08 against `d20d5dd`, after the frontend and
OpenTelemetry dependency updates landed on `main`.

## Decision

Adopt Oxlint as a fast frontend lint gate. Enable its default correctness
rules and these seven zero-debt rules from the evaluated policy:

- `oxc/no-accumulating-spread`
- `anti-slop/no-reduce-accumulator-copy`
- `anti-slop/no-object-parameters`
- `anti-slop/no-reflect-apply`
- `anti-slop/no-reflect-get`
- `anti-slop/no-unknown-type-aliases`
- `anti-slop/no-widen-then-assert`

Do not enable the complete anti-slop preset in CI yet. It reports 379 errors
across 81 files. Turning those rules into a gate would either block unrelated
work or require a broad cleanup whose behavior and value have not been
reviewed. The complete preset remains reproducible with
`npm run lint:anti-slop:evaluate`; a non-zero exit is expected until its
findings are resolved or its policy is narrowed deliberately.

Prettier remains the formatting authority. `svelte-check` and the Playwright
TypeScript project remain the type checks. Oxlint supplements those tools; it
does not replace them.

## Inputs

- Oxlint: `1.82.0`, exact version
- `@oxlint/plugins`: `1.82.0`, exact version matched to Oxlint
- anti-slop source: https://github.com/dmmulroy/anti-slop
- anti-slop commit: `95a56e5d24fb3d849673c2d51eb0908b8bd2d33b`
- anti-slop license: MIT

The plugin source is vendored under
`desktopexporter/internal/frontend/tools/oxlint/anti-slop/`. Its
`UPSTREAM.md` records the source revision and local deviations. Generated
Lezer parser files and the vendored plugin itself are excluded from linting.
The upstream installer copies production assets rather than RuleTester suites.
The six custom rules selected for the gate passed their upstream suites against
Oxlint and `@oxlint/plugins` 1.82.0 during this evaluation.

## Baseline

All commands ran from `desktopexporter/internal/frontend` on the same machine.
Times are single wall-clock observations, not benchmarks.

| Check | Result | Wall time |
| --- | --- | ---: |
| `npm run check` | 0 errors, 0 warnings | 3.95s |
| `npx tsc --project tsconfig.playwright.json` | pass | 0.75s |
| `npm run format:check` | pass | 2.60s |
| `npm run lint` | 0 errors, 20 warnings, 242 files | 0.38s |
| Complete anti-slop preset | 379 errors, 20 native warnings, 242 files | 0.81s |

The 20 native warnings are existing findings: 12 unused declarations, three
`no-new-array`, three `no-useless-spread`, one `no-useless-escape`, and one
`no-unused-expressions`. They remain visible but do not fail the gate. A future
cleanup can make warnings fatal without mixing that work into tool adoption.

Adding the pinned lint dependencies changed the lockfile and therefore the
output checked by `make build-ts-check`. The embedded frontend bundle was
regenerated from that lockfile; no application source changed.

## Complete preset findings

| Rule | Findings |
| --- | ---: |
| `require-safety-comment-for-type-assertion` | 168 |
| `no-runtime-typeof` | 58 |
| `no-known-value-widening` | 48 |
| `no-shape-in-symbol-names` | 26 |
| `no-unknown-parameters` | 22 |
| `no-chained-type-assertions` | 19 |
| `no-unsafe-dictionary-type` | 14 |
| `no-module-mocking` | 13 |
| `no-array-filter-map` | 5 |
| `no-unknown-returns` | 4 |
| `no-conditional-empty-object-spread` | 2 |

The largest groups need policy review before code changes. Type assertions are
common at DOM, chart-library, generated-type, and Svelte state boundaries.
Runtime `typeof` checks also appear inside the project's boundary-validation
functions, where anti-slop's optional type-guard allowance may be more
appropriate than a blanket prohibition. Module-mocking findings imply test
architecture changes rather than lint-only edits. Symbol names containing
`shape` need semantic review because some describe domain concepts rather than
implementation placeholders.

Anti-slop uses syntax and same-file lexical scope rather than the TypeScript
type checker. Its diagnostics are policy prompts, not proof that a reported
construct is unsafe or inefficient. Each deferred rule should be judged on a
representative sample before it becomes a gate.
