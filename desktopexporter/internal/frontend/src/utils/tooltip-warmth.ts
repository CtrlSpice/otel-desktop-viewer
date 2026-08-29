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

  function reset() {
    clearTimeout(coolTimer)
    coolTimer = undefined
    warm = false
    setInstant(false)
  }

  function destroy() {
    clearTimeout(coolTimer)
  }

  return { enter, leave, reset, destroy }
}

const TOOLTIP_SELECTOR = '.tooltip[data-tip]'
const INSTANT_PROPERTY = '--tooltip-show-delay'
const SUPPRESSED_ATTRIBUTE = 'data-tooltip-suppressed'

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

  const pointerActive = new Set<Element>()
  const focusActive = new Set<Element>()

  function pruneDetached(active: Set<Element>) {
    for (const tooltip of active) {
      if (tooltip.isConnected) continue
      active.delete(tooltip)
      warmth.leave()
    }
  }

  function pruneDetachedTooltips() {
    pruneDetached(pointerActive)
    pruneDetached(focusActive)
  }

  function tooltipForEvent(e: Event): Element | null {
    return (e.target as Element | null)?.closest(TOOLTIP_SELECTOR) ?? null
  }

  function stayedInside(e: Event, tooltip: Element): boolean {
    const related = (e as FocusEvent | PointerEvent).relatedTarget
    return related instanceof Node && tooltip.contains(related)
  }

  function enter(e: Event, active: Set<Element>) {
    pruneDetachedTooltips()
    const tooltip = tooltipForEvent(e)
    if (!tooltip || stayedInside(e, tooltip) || active.has(tooltip)) return

    if (!pointerActive.has(tooltip) && !focusActive.has(tooltip)) {
      tooltip.removeAttribute(SUPPRESSED_ATTRIBUTE)
    }
    active.add(tooltip)
    warmth.enter()
  }

  function leave(e: Event, active: Set<Element>) {
    pruneDetachedTooltips()
    const tooltip = tooltipForEvent(e)
    if (!tooltip || stayedInside(e, tooltip) || !active.delete(tooltip)) return

    warmth.leave()
    if (!pointerActive.has(tooltip) && !focusActive.has(tooltip)) {
      tooltip.removeAttribute(SUPPRESSED_ATTRIBUTE)
    }
  }

  const onPointerOver = (e: Event) => enter(e, pointerActive)
  const onPointerOut = (e: Event) => leave(e, pointerActive)
  const onFocusIn = (e: Event) => enter(e, focusActive)
  const onFocusOut = (e: Event) => leave(e, focusActive)
  const onKeyDown = (e: Event) => {
    pruneDetachedTooltips()
    const keyboardEvent = e as KeyboardEvent
    if (keyboardEvent.key !== 'Escape') return

    const tooltip = tooltipForEvent(e)
    if (!tooltip) return
    tooltip.setAttribute(SUPPRESSED_ATTRIBUTE, '')
    warmth.reset()
  }

  // pointerover/pointerout bubble and cover mouse + touch/pen; focusin/
  // focusout bubble (unlike focus/blur) and cover keyboard navigation, which
  // is how daisyUI shows tooltips for :focus-visible triggers too.
  root.addEventListener('pointerover', onPointerOver)
  root.addEventListener('pointerout', onPointerOut)
  root.addEventListener('focusin', onFocusIn)
  root.addEventListener('focusout', onFocusOut)
  root.addEventListener('keydown', onKeyDown)

  return () => {
    root.removeEventListener('pointerover', onPointerOver)
    root.removeEventListener('pointerout', onPointerOut)
    root.removeEventListener('focusin', onFocusIn)
    root.removeEventListener('focusout', onFocusOut)
    root.removeEventListener('keydown', onKeyDown)
    for (const tooltip of root.querySelectorAll(TOOLTIP_SELECTOR)) {
      tooltip.removeAttribute(SUPPRESSED_ATTRIBUTE)
    }
    if (root instanceof Element && root.matches(TOOLTIP_SELECTOR)) {
      root.removeAttribute(SUPPRESSED_ATTRIBUTE)
    }
    warmth.destroy()
    target.style.removeProperty(INSTANT_PROPERTY)
  }
}
