import { render } from 'svelte/server'
import Results from './Results.svelte'
import type { Presentation } from './model.ts'

export function renderResults(data: Presentation) {
  // This is a static export: Svelte's hydration markers are unnecessary.
  return render(Results, { props: { data } }).body.replace(/<!--.*?-->/gs, '')
}
