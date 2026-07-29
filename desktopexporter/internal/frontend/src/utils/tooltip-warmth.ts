// Tooltips wait before their first appearance, then show instantly while
// "warm" (macOS/VS Code/GitHub pattern). CSS can't remember "a tooltip was
// just visible" once the pointer leaves, so that memory lives here; the
// actual delay value lives in --tooltip-show-delay (app.css).

export const TOOLTIP_WARM_GRACE_MS = 300

/** Framework-agnostic state machine: warms on enter, cools after a grace
 * period with no tooltip trigger hovered or focused. `setInstant` is the only
 * side effect -- call sites decide what "instant" means (a test can just
 * record calls; the real wiring flips a CSS custom property). */
export function createTooltipWarmth(
  setInstant: (instant: boolean) => void,
  graceMs: number = TOOLTIP_WARM_GRACE_MS
) {
  let warm = false
  let coolTimer: ReturnType<typeof setTimeout> | undefined
  // Counts concurrent triggers rather than tracking a single "active"
  // element, so a pointer and a keyboard focus landing on different
  // triggers at once don't cool each other's grace period prematurely.
  let activeCount = 0

  function enter() {
    activeCount += 1
    clearTimeout(coolTimer)
    coolTimer = undefined
    if (!warm) {
      warm = true
      setInstant(false)
    } else {
      setInstant(true)
    }
  }

  function leave() {
    activeCount = Math.max(0, activeCount - 1)
    if (activeCount > 0) return
    clearTimeout(coolTimer)
    coolTimer = setTimeout(() => {
      warm = false
      setInstant(false)
    }, graceMs)
  }

  function destroy() {
    clearTimeout(coolTimer)
  }

  return { enter, leave, destroy }
}

const TOOLTIP_SELECTOR = '.tooltip[data-tip]'
const INSTANT_PROPERTY = '--tooltip-show-delay'

/** Wires the controller to real pointer/focus events via delegation on
 * `root`, so newly-mounted tooltips are covered with no per-component setup.
 * Call once at app startup; call the returned cleanup on teardown (tests
 * only -- the app itself never tears this down). */
export function initTooltipWarmth(
  root: Document | HTMLElement = document,
  target: HTMLElement = document.documentElement
) {
  const warmth = createTooltipWarmth(instant => {
    if (instant) {
      target.style.setProperty(INSTANT_PROPERTY, '0s')
    } else {
      target.style.removeProperty(INSTANT_PROPERTY)
    }
  })

  const onOver = (e: Event) => {
    const el = (e.target as Element | null)?.closest(TOOLTIP_SELECTOR)
    if (el) warmth.enter()
  }
  const onOut = (e: Event) => {
    const el = (e.target as Element | null)?.closest(TOOLTIP_SELECTOR)
    if (el) warmth.leave()
  }

  // pointerover/pointerout bubble and cover mouse + touch/pen; focusin/
  // focusout bubble (unlike focus/blur) and cover keyboard navigation, which
  // is how daisyUI shows tooltips for :focus-visible triggers too.
  root.addEventListener('pointerover', onOver)
  root.addEventListener('pointerout', onOut)
  root.addEventListener('focusin', onOver)
  root.addEventListener('focusout', onOut)

  return () => {
    root.removeEventListener('pointerover', onOver)
    root.removeEventListener('pointerout', onOut)
    root.removeEventListener('focusin', onOver)
    root.removeEventListener('focusout', onOut)
    warmth.destroy()
    target.style.removeProperty(INSTANT_PROPERTY)
  }
}
