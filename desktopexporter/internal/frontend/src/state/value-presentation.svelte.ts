import { SvelteSet } from 'svelte/reactivity'

// Display choices are scoped to a field's provenance, never its received value.
const shellFields = new SvelteSet<string>()

export function valuePresentationKey(
  source: string,
  kind: string,
  key: string
) {
  return JSON.stringify([source, kind, key])
}

export const valuePresentation = {
  isShell(key: string) {
    return shellFields.has(key)
  },
  setShell(key: string, enabled: boolean) {
    if (enabled) shellFields.add(key)
    else shellFields.delete(key)
  },
}
