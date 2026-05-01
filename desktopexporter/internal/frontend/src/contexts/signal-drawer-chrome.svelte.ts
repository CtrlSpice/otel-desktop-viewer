import { getContext } from 'svelte'

export const SIGNAL_DRAWER_CHROME_KEY = Symbol('signal-drawer-chrome')

/** Set on SignalListDrawer; read in DrawerSearchPanel for theme + close placement. */
export type SignalDrawerChrome = {
  closeForId: string | undefined
}

export function getSignalDrawerChrome(): SignalDrawerChrome | undefined {
  return getContext(SIGNAL_DRAWER_CHROME_KEY)
}
