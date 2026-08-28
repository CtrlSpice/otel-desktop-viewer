/**
 * The width rules every side panel shares: one default, one floor, one
 * ceiling, all in rem.
 *
 * @remarks
 * Two panels flank the main view -- the signal list drawer and the detail
 * panel -- and they follow the same rules so "the same width" means what it
 * says. The numbers are derived, not picked:
 *
 * - The floor comes from the tightest content either panel holds: the detail
 *   pane's tab strip. `PaneHeader.svelte` floors each tab at
 *   `--pane-tab-min: 7rem`, so three tabs need 336px, plus 6px of chrome
 *   (1px border each side of `.page-layout__detail-chrome`, 2px padding each
 *   side of `.pane-header`) = 342px. 22rem is the clean value above it.
 *   If `--pane-tab-min` ever moves, this floor must move with it.
 * - The default is the width the drawer has always had.
 * - The ceiling is the layout's, not the content's -- content has no upper
 *   bound (Kubernetes names run arbitrarily long). It is what a 1440px
 *   viewport leaves after the main pane's floor (28rem) and the other
 *   panel's floor (22rem): 40rem.
 *
 * Rem rather than px throughout so panels keep their proportion to the text
 * they hold when the root font size changes; conversion reads the live root
 * font size rather than assuming 16.
 */

export const PANEL_DEFAULT_REM = 28
/** >= 3 x --pane-tab-min + 6px chrome; see the derivation above. */
export const PANEL_MIN_REM = 22
export const PANEL_MAX_REM = 40

function rootFontPx(): number {
  if (typeof document === 'undefined') return 16
  const px = Number.parseFloat(
    getComputedStyle(document.documentElement).fontSize
  )
  return Number.isFinite(px) && px > 0 ? px : 16
}

export function remToPx(rem: number): number {
  return rem * rootFontPx()
}

/**
 * Clamp a panel width into the shared bounds. Non-numbers fall back to the
 * default rather than propagating: a NaN width renders a broken panel, and
 * the callers feed this stored values that can be anything.
 */
export function clampPanelRem(rem: number): number {
  if (!Number.isFinite(rem)) return PANEL_DEFAULT_REM
  return Math.min(PANEL_MAX_REM, Math.max(PANEL_MIN_REM, rem))
}

/**
 * A rem width as a fraction of a measured container -- for panels whose drag
 * state is a fraction (the main/detail split) but whose default is absolute,
 * so "the same width" holds at every window size (R2). The caller supplies
 * the container it actually distributes: for a flex split that is the flex
 * space, not the raw container.
 */
export function remToFraction(rem: number, containerPx: number): number {
  if (containerPx <= 0) return 0
  return remToPx(rem) / containerPx
}
