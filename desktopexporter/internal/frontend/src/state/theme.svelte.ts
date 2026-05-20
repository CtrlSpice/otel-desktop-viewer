// Reactive view of the current theme. The ThemeToggle component flips
// `data-theme` on <html> to switch palettes -- anything that needs to
// re-derive when that happens (chart palettes, computed swatches, etc.)
// can read `themeSignal.value` inside a $derived and Svelte will
// re-evaluate on theme change without that component owning a
// MutationObserver.
//
// Implementation: createSubscriber wires a MutationObserver on the html
// element. The subscriber is shared across all readers (one observer for
// the page) and only attaches while at least one consumer is listening.

import { createSubscriber } from 'svelte/reactivity'

function readDataTheme(): string {
  if (typeof document === 'undefined') return ''
  return document.documentElement.getAttribute('data-theme') ?? ''
}

const subscribe = createSubscriber(update => {
  if (typeof document === 'undefined') return
  const observer = new MutationObserver(() => update())
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-theme'],
  })
  return () => observer.disconnect()
})

/**
 * `themeSignal.value` returns the current `data-theme` string and
 * registers the calling reactive scope as a dependency, so it will
 * re-run when the theme changes.
 */
export const themeSignal = {
  get value(): string {
    subscribe()
    return readDataTheme()
  },
}
