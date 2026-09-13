# Oxlint and anti-slop evaluation

Evaluated on 2026-09-08 against `d20d5dd`, after the frontend and
OpenTelemetry dependency updates landed on `main`.

## Cleanup progress (2026-09-13)

The original evaluation below is retained as the adoption baseline. Standard
lint now rejects warnings and passes with zero warnings and errors. The naming
slice on top of `bc0ffcf` clears all 26 `no-shape-in-symbol-names` diagnostics:
waterfall search-collapse state, metric aggregation cases, and accessibility
control measurements now have role-specific names. The rule is enabled as an
error in standard lint.

Previously cleared rules also promoted to the standard gate are
`no-array-filter-map`, `no-chained-type-assertions`,
`no-conditional-empty-object-spread`, and `no-unknown-returns`.

The complete preset now reports **78 diagnostics**:

| Rule | Remaining findings |
| --- | ---: |
| `require-safety-comment-for-type-assertion` | 34 |
| `no-known-value-widening` | 1 |
| `no-runtime-typeof` | 24 |
| `no-unsafe-dictionary-type` | 12 |
| `no-module-mocking` | 5 |
| `no-unknown-parameters` | 2 |
| `no-shape-in-symbol-names` | 0 |

The service/bigint source slice resolved 18 diagnostics from the 138-count
snapshot: nine runtime representation checks, six unknown parameters, two
unjustified assertions, and one unsafe test dictionary. PR #485 then resolved
four module-mocking findings, taking the source-fix total from 120 to 116.

The move from 116 to 108 is policy reclassification, not eight more code fixes.
`no-runtime-typeof` remains an evaluation error but now uses
`allowInTypeGuards: true`, so eight `typeof` checks inside truthful TypeScript
predicates are accepted. Non-predicate checks still report 24 errors. Do not
promote this rule to standard lint until that count reaches zero.

After that policy change, PR #486 resolved four more module-mocking findings,
taking the total from 108 to 104. This is another source fix; it does not change
the eight-finding policy reclassification above.

The finite lookup and correlated search-result slices resolve 26 more source
findings from that latest-main count: 25 known-value widenings and one search
result assertion. The remaining `no-known-value-widening` finding belongs to
persistence parsing, so that rule remains evaluation-only rather than joining
the standard lint gate.

Next slices address finite lookups, inferred or named contracts, DOM narrowing,
and typed component harnesses. Persistence parsing must preserve salvage
semantics; trusted backend RPC assertions need precise boundary evidence.

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
