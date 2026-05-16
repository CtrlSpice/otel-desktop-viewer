/*
 * Drawer open preference is persisted in localStorage, but each signal
 * route mounts its own SignalListDrawer. Without this, the panel width
 * transition runs on every navigation even when open/closed did not change.
 */

let lastOpen: boolean | undefined

/** Skip the width tween when remounting with the same open preference. */
export function shouldSkipDrawerWidthTransition(open: boolean): boolean {
  const skip = lastOpen !== undefined && lastOpen === open
  lastOpen = open
  return skip
}

export function syncDrawerOpenPreference(open: boolean): void {
  lastOpen = open
}
