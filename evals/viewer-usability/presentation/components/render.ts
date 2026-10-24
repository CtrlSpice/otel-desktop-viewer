import { render } from 'svelte/server'
import Results from './Results.svelte'
import type { Presentation } from './model.ts'

export function renderResults(data: Presentation) {
  return render(Results, { props: { data } }).body
}
