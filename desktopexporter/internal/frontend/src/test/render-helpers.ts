import { render } from '@testing-library/svelte'
import type { Component, ComponentProps } from 'svelte'
import ContextHarness from './ContextHarness.svelte'

type ContextHarnessUpdate<T extends Component<any>> = {
  component?: T
  componentProps?: ComponentProps<T>
}

/**
 * Renders a component inside route + time contexts.
 *
 * Set the URL with {@link setTestUrl} BEFORE calling this: both contexts read
 * the live URL at creation time.
 */
export function renderWithContexts<T extends Component<any>>(
  component: T,
  props?: ComponentProps<T>
) {
  const view = render(ContextHarness, {
    props: { component, componentProps: props },
  })
  return {
    ...view,
    rerender(update: ContextHarnessUpdate<T>) {
      return view.rerender(update)
    },
  }
}

/** Points the jsdom URL at `path` without adding a history entry. */
export function setTestUrl(path: string): void {
  window.history.replaceState(null, '', path)
}
