<!--
Keep the pull request focused and remove inapplicable sections.
Maintainers use issues to understand problems and agree on substantial
implementation approaches before code is submitted.
-->

## Why

<!-- What problem does this solve, and why is this change needed now? -->

Related issue:

## Alignment

<!--
For substantial user-facing, architectural, data-model, persistence, or
cross-cutting work, link the issue discussion or design where a maintainer
agreed to this approach.
For a small bug fix, documentation change, or narrow polish, write "N/A".
Use "Fixes" or "Closes" after the implementation matches the agreed issue scope.
-->

Agreed approach:

## What changed

<!-- Describe the behavior and implementation. Call out important trade-offs. -->

-

## Verification

### Targeted tests

<!--
Name the tests that exercise this change directly and the behavior they pin.
Include focused regression tests alongside broad suite results. When the current
harness lacks a test seam, link the maintainer discussion and include the seam
in this change.
-->

-

### Commands

<!-- Include the exact commands run and their results. `make test` is preferred. -->

```text

```

### Manual verification

<!-- Describe concrete checks in the running application, or write "N/A". -->

-

## Interface evidence

<!--
For interface changes, include before/after screenshots or a short recording
with a text description. Record the visual and assistive checks performed,
including relevant responsive layouts, themes, zoom, keyboard operation, focus,
screen-reader names, visible alternatives for audio feedback, reduced motion,
and loading, empty, or error states. Write "N/A" for other changes.
-->

## Generated artifacts

<!--
- Frontend changes: run `make build-ts` and commit `internal/server/static`.
- Frontend dependency changes: commit the manifest, lockfile, and rebuilt assets.
- Search grammar changes: regenerate and commit the parser output.
- Otherwise write "N/A".
Resolve source conflicts first and regenerate bundles from the combined source.
-->

## Checklist

- [ ] The change is focused and contains only related work.
- [ ] I linked maintainer alignment for substantial work, or this is a narrow bug fix, documentation change, or polish.
- [ ] I added or updated focused tests for every changed behavior.
- [ ] I verified visual and assistive behavior for interface changes.
- [ ] I reviewed and understand every submitted change, including generated or agent-assisted code.
- [ ] I updated relevant documentation and comments.
- [ ] I included current generated artifacts where required.
