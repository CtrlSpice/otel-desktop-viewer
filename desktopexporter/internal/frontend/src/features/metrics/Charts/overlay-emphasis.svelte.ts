import { SvelteSet } from 'svelte/reactivity'

/** Presentation-only emphasis; overlay visibility and computed values stay separate. */
export function createOverlayEmphasis() {
  const active = new SvelteSet<string>()
  return {
    has: (id: string) => active.has(id),
    toggle(id: string) {
      if (!active.delete(id)) active.add(id)
    },
  }
}
