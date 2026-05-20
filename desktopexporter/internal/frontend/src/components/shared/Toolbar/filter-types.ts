import type { Snippet } from 'svelte'

/** Toolbar / search filter: icon button + popover body. */
export type FilterDescriptor = {
  id: string
  label: string
  icon: Snippet
  content: Snippet
}
