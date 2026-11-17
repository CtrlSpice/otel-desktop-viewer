type ChartScrollDiscoveryOptions = {
  visit: string | undefined
  overflows: boolean
}

/** Keep the native scrollbar visible until scrolling is discovered this visit. */
export function chartScrollDiscovery(
  viewport: HTMLElement,
  options: ChartScrollDiscoveryOptions
) {
  let discovered = false
  const updateClass = () =>
    viewport.classList.toggle(
      'chart-scroll-discoverable',
      options.overflows && !discovered
    )
  const observe = () =>
    observeChartScrollDiscovery(viewport, () => {
      discovered = true
      updateClass()
    })
  let stopObserving = observe()
  updateClass()

  return {
    update(next: ChartScrollDiscoveryOptions) {
      if (next.visit !== options.visit) {
        stopObserving()
        discovered = false
        stopObserving = observe()
      }
      options = next
      updateClass()
    },
    destroy() {
      stopObserving()
      viewport.classList.remove('chart-scroll-discoverable')
    },
  }
}

/** Observe actual horizontal movement following user input. Scroll events alone
 * also fire for layout, restored positions and keyboard focus, so are not proof
 * that the user has discovered scrolling. The plot host includes the scroller
 * and its sibling keyboard surface. This observer never changes either one. */
function observeChartScrollDiscovery(
  viewport: HTMLElement,
  onDiscover: () => void
) {
  const controller = new AbortController()
  const options = { passive: true, signal: controller.signal }
  let pointer: number | null = null
  let moved = false
  let lastScrollLeft = viewport.scrollLeft
  let intent: {
    left: number
    width: number
    contentWidth: number
    until: number
  } | null = null

  function finish() {
    controller.abort()
    onDiscover()
  }

  function arm(event: Event, left = viewport.scrollLeft) {
    if (!event.isTrusted) return
    const now = performance.now()
    if (!intent || now > intent.until) {
      intent = {
        left,
        width: viewport.clientWidth,
        contentWidth: viewport.scrollWidth,
        until: now,
      }
    }
    intent.until = now + 500
  }

  viewport.addEventListener(
    'wheel',
    event => {
      if (
        !event.ctrlKey &&
        (event.deltaX !== 0 || (event.shiftKey && event.deltaY !== 0))
      )
        // Compositor scrolling can precede a passive wheel callback. Compare
        // against the last scroll event, rather than the already-moved viewport.
        arm(event, lastScrollLeft)
    },
    options
  )

  viewport.addEventListener(
    'pointerdown',
    event => {
      // Touch pans and native scrollbar drags count; clicking a bar does not.
      if (
        event.isTrusted &&
        (event.pointerType === 'touch' || event.target === viewport)
      ) {
        pointer = event.pointerId
        arm(event)
      }
    },
    options
  )

  const releasePointer = (event: PointerEvent) => {
    if (event.pointerId !== pointer) return
    pointer = null
    // Do not replace a native scrollbar while its thumb is being dragged.
    if (moved) {
      finish()
      return
    }
    if (intent) intent.until = performance.now() + 500
  }
  window.addEventListener('pointerup', releasePointer, options)
  window.addEventListener('pointercancel', releasePointer, options)

  viewport.parentElement?.addEventListener(
    'keydown',
    event => {
      if (
        !event.altKey &&
        event.target instanceof HTMLElement &&
        (event.target === viewport ||
          event.target.classList.contains('chart-keyboard-surface')) &&
        ['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)
      )
        arm(event)
    },
    { ...options, capture: true }
  )

  viewport.addEventListener(
    'scroll',
    () => {
      lastScrollLeft = viewport.scrollLeft
      if (!intent) return
      if (
        (pointer === null && performance.now() > intent.until) ||
        viewport.clientWidth !== intent.width ||
        viewport.scrollWidth !== intent.contentWidth
      ) {
        intent = null
        return
      }
      if (Math.abs(viewport.scrollLeft - intent.left) <= 1) return
      moved = true
      if (pointer === null) finish()
    },
    options
  )

  return () => controller.abort()
}
