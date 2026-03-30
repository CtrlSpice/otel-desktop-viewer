/**
 * Lightweight helpers for stashing/retrieving page state via the History API.
 *
 * tinro always pushes `{}` as the state object, so we layer our own state
 * on top of the *current* history entry with `replaceState`.  When the user
 * navigates back (history.back()), the browser restores the entry — state
 * and all — so the originating page can pick it up on re-mount.
 */

/** Merge a keyed value into the current history entry. */
export function stashNavState<T>(key: string, value: T): void {
  const current = history.state ?? {}
  history.replaceState({ ...current, [key]: value }, '')
}

/** Read (and optionally remove) a keyed value from the current history entry. */
export function popNavState<T>(key: string): T | undefined {
  const state = history.state
  if (!state || !(key in state)) return undefined

  const value = state[key] as T

  const { [key]: _, ...rest } = state
  history.replaceState(rest, '')

  return value
}
