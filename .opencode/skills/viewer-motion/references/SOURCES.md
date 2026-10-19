# Design resources

Selected 2026-10-19. The downloaded files are unmodified upstream references.

## Emil Kowalski — animation craft

- Repository: https://github.com/emilkowalski/skills
- Revision: `2d228630ba1446365f614a108cb8252f1feda202`
- `emil/animate.md`: upstream `skills/animate/SKILL.md`
- `emil/RECIPES.md`: upstream `skills/animate/RECIPES.md`
- License: MIT; see `emil/LICENSE`.
- Best fit: deciding when to animate, short durations, interruption, easing,
  reduced motion, and practical implementations.
- Companion article: https://emilkowal.ski/ui/great-animations

## Paul Bakaus / Impeccable — product UI and polish

- Repository: https://github.com/pbakaus/impeccable
- Revision: `46b822f7dda9f7dba799a9c7776fdc19b3a93559`
- `impeccable/operate.md`: upstream `.opencode/skills/impeccable/reference/operate.md`
- `impeccable/polish.md`: upstream `.opencode/skills/impeccable/reference/polish.md`
- License: Apache-2.0; see `impeccable/LICENSE`.
- Best fit: hierarchy, consistency, restrained motion, and polishing an existing
  application without replacing its visual identity.

## Implementation references

- Animation performance: https://web.dev/articles/animations-guide
- Svelte transitions: https://svelte.dev/docs/svelte/transition
- Reduced motion: https://svelte.dev/docs/svelte/svelte-motion#prefersReducedMotion
- Existing button system: https://daisyui.com/components/button/

The project-local `Viewer Motion` skill supplies the app-specific direction.
Upstream recipes that use other frameworks must be adapted to Svelte and the
existing shared components.
