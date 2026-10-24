<script lang="ts">
  import { onDestroy, untrack } from 'svelte'
  import {
    remToFraction,
    remToPx,
    PANEL_CLOSE_OVERSHOOT_REM,
    resolvePanelResize,
  } from '@/state/panel-width'
  import { startDrag, type DragHandle } from './utils/drag'
  import PanelClip from './PanelClip.svelte'

  /** Default split when no prop / storage; keep in sync with prop default below */
  const DEFAULT_LEFT_WIDTH = 0.7

  type Props = {
    leftPanel: any
    rightPanel: any
    /** Keep the detail mounted while the main pane fills the available space. */
    rightCollapsed?: boolean
    onRightCollapsedChange?: (collapsed: boolean) => void
    rightPanelID?: string
    /** Stacked layout only: use the bottom panel header as the resize
     *  handle instead of a separate divider strip. */
    defaultLeftWidth?: number
    /** Absolute default for the right pane, in rem. A fraction default
     *  gives the pane a different pixel width at every window size;
     *  this holds "the same width" across windows -- and across panels,
     *  since the drawer's default is also rem. Wins over
     *  defaultLeftWidth once the container is measured; a stored split
     *  still beats both. Ignored while stacked (it is a width, and the
     *  stacked split divides height). */
    defaultRightRem?: number
    /** Shared horizontal width. Stacked splits keep their own height fraction. */
    rightWidthRem?: number
    onRightWidthChange?: (rem: number) => void
    /** Initialize saved sizing using measured usable space, before applying it. */
    onLayoutMeasure?: (flexSpacePx: number, stacked: boolean) => void
    /** Minimum left fraction of the container (0..1). */
    minLeftWidth?: number
    /** Minimum right fraction of the container (0..1). */
    minRightWidth?: number
    /** Optional absolute pixel floor for the left pane. When set,
     *  the drag clamps to MAX(fraction floor, pixel floor). Lets
     *  callers guarantee enough room for fixed-size chrome (e.g.
     *  a tab strip) regardless of viewport width. */
    minLeftPx?: number
    /** Optional absolute pixel floor for the right pane. */
    minRightPx?: number
    /** Optional absolute pixel ceiling for the left pane. */
    maxLeftPx?: number
    /** Optional absolute pixel ceiling for the right pane. */
    maxRightPx?: number
    storageKey?: string
    stackBreakpoint?: number
    /** Re-measure destination slots before paint when the app route changes. */
    layoutKey?: string
  }

  let {
    leftPanel,
    rightPanel,
    rightCollapsed = false,
    onRightCollapsedChange,
    rightPanelID,
    defaultLeftWidth = DEFAULT_LEFT_WIDTH,
    defaultRightRem,
    rightWidthRem,
    onRightWidthChange,
    onLayoutMeasure,
    minLeftWidth = 0.3,
    minRightWidth = 0.2,
    minLeftPx,
    minRightPx,
    maxLeftPx,
    maxRightPx,
    storageKey,
    stackBreakpoint = 800,
    layoutKey,
  }: Props = $props()

  let leftWidth = $state(DEFAULT_LEFT_WIDTH)
  let appliedInitialDefault = $state(false)
  let isDragging = $state(false)
  let dragLeftWidth = $state<number | null>(null)
  const renderedLeftWidth = $derived(dragLeftWidth ?? leftWidth)

  let containerRef = $state<HTMLDivElement | null>(null)
  let dividerRef = $state<HTMLElement | null>(null)
  let containerWidth = $state(0)
  let containerHeight = $state(0)
  let skipWidthTransition = $state(true)

  // Route changes replace contents, not the layout. Apply the first measured
  // size before enabling the transitions used by explicit collapse/reopen.
  $effect(() => {
    if (containerWidth <= 0 || !skipWidthTransition) return
    let frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() => {
        skipWidthTransition = false
      })
    })
    return () => cancelAnimationFrame(frame)
  })

  /** Matches CSS `gap` on the flex container (`--panel-split-flex-gap`). */
  function panelSplitGapPx(): number {
    if (!containerRef) return 8
    const s = getComputedStyle(containerRef)
    const raw = stacked ? s.rowGap : s.columnGap
    const px = parseFloat(raw)
    return Number.isFinite(px) ? px : 8
  }

  let stacked = $derived(containerWidth > 0 && containerWidth < stackBreakpoint)

  /* The space the two panes actually divide: the container minus the
     divider and the two flex gaps. Every pixel-to-fraction conversion
     in this component must use this, not the raw container -- the
     fractions are flex-grow shares of exactly this space, so a value
     converted against the container under-delivers by the divider and
     gaps (~24px, which took a 352px floor to ~347 rendered). */
  function flexSpacePx(): number {
    const raw = stacked ? containerHeight : containerWidth
    if (raw <= 0) return 0
    // Reserve the full gutter while it animates closed. This app-shell token
    // is in rem; measuring the shrinking divider would change the saved split.
    const gutterRem = containerRef
      ? parseFloat(
          getComputedStyle(containerRef).getPropertyValue('--panel-split-total')
        )
      : NaN
    if (Number.isFinite(gutterRem)) return Math.max(1, raw - remToPx(gutterRem))
    const divSize = dividerRef
      ? stacked
        ? dividerRef.offsetHeight
        : dividerRef.offsetWidth
      : 0
    return Math.max(1, raw - divSize - 2 * panelSplitGapPx())
  }

  const rightSize = $derived((1 - renderedLeftWidth) * flexSpacePx())

  /* The default this split starts at and resets to. The rem form wins
     once the container is measured, so the *rendered* right pane lands
     on the rem width. While stacked the split divides height, so a
     width in rem does not apply and the fraction default holds. */
  function resolvedDefaultLeft(): number {
    if (defaultRightRem == null || stacked || containerWidth <= 0) {
      return defaultLeftWidth
    }
    return 1 - remToFraction(defaultRightRem, flexSpacePx())
  }

  $effect.pre(() => {
    if (containerWidth > 0) onLayoutMeasure?.(flexSpacePx(), stacked)
    if (rightCollapsed) return
    if (rightWidthRem != null && !stacked) {
      if (containerWidth <= 0) return
      leftWidth = Math.max(
        effectiveMinLeft,
        Math.min(
          effectiveMaxLeft,
          1 - remToFraction(rightWidthRem, flexSpacePx())
        )
      )
      return
    }
    if (appliedInitialDefault) return
    // A rem default cannot be converted before the container reports a
    // width; hold the fraction fallback until the first measurement so
    // the first applied default is the right one.
    if (defaultRightRem != null && containerWidth <= 0) return
    // A stored split owns initialization. Because this effect waits for
    // the measurement, it fires after the storage effect has already
    // applied the saved value -- writing the default here would clobber
    // it on every load.
    const stored = storageKey ? localStorage.getItem(storageKey) : null
    if (stored === null || Number.isNaN(Number.parseFloat(stored))) {
      leftWidth = resolvedDefaultLeft()
    }
    appliedInitialDefault = true
  })

  /* Pixel floors/ceilings → fractions of the flex space (width when
     side-by-side, height when stacked). Graceful fallback when both
     pixel floors exceed the container: prefer the right pane's floor
     (detail strip / timeseries list). */
  let splitBounds = $derived.by(() => {
    const dim = flexSpacePx()
    if (dim <= 0) {
      return {
        minLeft: minLeftWidth,
        minRight: minRightWidth,
        maxLeft: 1 - minRightWidth,
      }
    }

    const leftPxFrac = minLeftPx ? minLeftPx / dim : 0
    const rightPxFrac = minRightPx ? minRightPx / dim : 0
    let minLeft = Math.max(minLeftWidth, leftPxFrac)
    let minRight = Math.max(minRightWidth, rightPxFrac)

    if (maxRightPx) {
      minLeft = Math.max(minLeft, 1 - maxRightPx / dim)
    }

    let maxLeft = 1 - minRight
    if (maxLeftPx) {
      maxLeft = Math.min(maxLeft, maxLeftPx / dim)
    }

    if (minLeft + minRight > 1) {
      if (minRight <= 1 - minLeftWidth) {
        minLeft = Math.max(minLeftWidth, 1 - minRight)
      } else {
        minLeft = minLeftWidth
        minRight = minRightWidth
        maxLeft = 1 - minRight
        if (maxLeftPx) maxLeft = Math.min(maxLeft, maxLeftPx / dim)
        if (maxRightPx) minLeft = Math.max(minLeft, 1 - maxRightPx / dim)
      }
    }

    if (minLeft > maxLeft) maxLeft = minLeft

    return { minLeft, minRight, maxLeft }
  })
  let effectiveMinLeft = $derived(splitBounds.minLeft)
  let effectiveMinRight = $derived(splitBounds.minRight)
  let effectiveMaxLeft = $derived(splitBounds.maxLeft)

  $effect(() => {
    if (rightCollapsed) return
    if (rightWidthRem != null && !stacked) return
    if (storageKey) {
      let saved = localStorage.getItem(storageKey)
      if (saved) {
        let parsed = parseFloat(saved)
        // Clamp persisted splits to the current viewport bounds.
        if (!isNaN(parsed)) {
          leftWidth = Math.max(
            effectiveMinLeft,
            Math.min(effectiveMaxLeft, parsed)
          )
        }
      }
    }
  })

  /* Re-clamp the current width whenever the effective minimums move.
     This catches the viewport-shrink case: if the user makes the
     window narrow enough that the current split would put one pane
     below its pixel floor, snap it back to the floor. */
  $effect(() => {
    if (rightCollapsed) return
    const lo = effectiveMinLeft
    const hi = effectiveMaxLeft
    if (lo > hi) return
    if (leftWidth < lo) leftWidth = lo
    else if (leftWidth > hi) leftWidth = hi
  })

  function saveWidth() {
    if (rightWidthRem != null && !stacked) {
      onRightWidthChange?.(((1 - leftWidth) * flexSpacePx()) / remToPx(1))
      return
    }
    if (storageKey) {
      localStorage.setItem(storageKey, leftWidth.toString())
    }
  }

  let drag: DragHandle | null = null

  function handlePointerDown(e: PointerEvent) {
    if (isDragging) return
    isDragging = true
    const startWidth = leftWidth
    // Pinned for the drag: a fraction moves per pixel of flex space,
    // and remeasuring mid-drag would make the mapping wobble.
    const flexSpace = flexSpacePx() || 1
    let desired = startWidth
    drag = startDrag(e, {
      axis: stacked ? 'y' : 'x',
      onMove: delta => {
        desired = startWidth + delta / flexSpace
        const clamped = Math.max(
          effectiveMinLeft,
          Math.min(effectiveMaxLeft, desired)
        )
        dragLeftWidth = onRightCollapsedChange
          ? 1 -
            resolvePanelResize(
              1 - desired,
              1 - effectiveMaxLeft,
              1 - effectiveMinLeft,
              remToPx(PANEL_CLOSE_OVERSHOOT_REM) / flexSpace
            ).preview
          : clamped
      },
      onEnd: () => {
        isDragging = false
        drag = null
        if (
          onRightCollapsedChange &&
          resolvePanelResize(
            1 - desired,
            1 - effectiveMaxLeft,
            1 - effectiveMinLeft,
            remToPx(PANEL_CLOSE_OVERSHOOT_REM) / flexSpace
          ).collapsed
        ) {
          onRightCollapsedChange(true)
        } else {
          leftWidth = Math.max(
            effectiveMinLeft,
            Math.min(effectiveMaxLeft, desired)
          )
          saveWidth()
        }
        dragLeftWidth = null
      },
    })
  }

  // A drag outliving its component would leave the body cursor and
  // text selection suppressed for the rest of the session.
  onDestroy(() => drag?.cancel())

  function handleDoubleClick() {
    leftWidth = resolvedDefaultLeft()
    saveWidth()
  }

  function handleKeydown(e: KeyboardEvent) {
    skipWidthTransition = true
    const step = e.shiftKey ? 0.05 : 0.01
    const lo = effectiveMinLeft
    const hi = effectiveMaxLeft
    if (stacked) {
      if (e.key === 'ArrowUp' && leftWidth > lo) {
        e.preventDefault()
        leftWidth = Math.max(lo, leftWidth - step)
        saveWidth()
      } else if (e.key === 'ArrowDown' && leftWidth < hi) {
        e.preventDefault()
        leftWidth = Math.min(hi, leftWidth + step)
        saveWidth()
      }
    } else {
      if (e.key === 'ArrowLeft' && leftWidth > lo) {
        e.preventDefault()
        leftWidth = Math.max(lo, leftWidth - step)
        saveWidth()
      } else if (e.key === 'ArrowRight' && leftWidth < hi) {
        e.preventDefault()
        leftWidth = Math.min(hi, leftWidth + step)
        saveWidth()
      }
    }
  }

  function measure(bounds: DOMRectReadOnly) {
    if (containerWidth !== bounds.width || containerHeight !== bounds.height)
      skipWidthTransition = true
    containerWidth = bounds.width
    containerHeight = bounds.height
  }

  $effect(() => {
    void layoutKey
    const element = containerRef
    if (element) untrack(() => measure(element.getBoundingClientRect()))
  })

  $effect(() => {
    const element = containerRef
    if (!element) return
    let frame: number | undefined
    const ro = new ResizeObserver(() => {
      // Responsive measurements can resize descendant observers. Coalesce
      // outside delivery; route changes use the before-paint measurement above.
      if (frame !== undefined) return
      frame = requestAnimationFrame(() => {
        frame = undefined
        measure(element.getBoundingClientRect())
      })
    })
    ro.observe(element)
    return () => {
      ro.disconnect()
      if (frame !== undefined) cancelAnimationFrame(frame)
    }
  })
</script>

<div
  class="panel-split flex h-full w-full"
  class:flex-col={stacked}
  class:panel-split--collapsed={rightCollapsed}
  class:panel-split--dragging={isDragging}
  class:panel-split--initializing={skipWidthTransition}
  bind:this={containerRef}
>
  <div
    class="panel-shell min-h-0 min-w-0 overflow-hidden {stacked &&
    !rightCollapsed
      ? 'rounded-t-xl rounded-b-none'
      : 'rounded-xl'}"
    style="flex: {stacked && !rightCollapsed ? renderedLeftWidth : 1} 1 0px"
  >
    {@render leftPanel()}
  </div>

  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    bind:this={dividerRef}
    class="col-resize-bar {stacked
      ? 'col-resize-bar--row-in-flow'
      : 'col-resize-bar--in-flow'}"
    inert={rightCollapsed}
    aria-hidden={rightCollapsed}
    class:col-resize-bar--active={isDragging}
    onpointerdown={handlePointerDown}
    ondblclick={handleDoubleClick}
    onkeydown={handleKeydown}
    role="separator"
    aria-orientation={stacked ? 'horizontal' : 'vertical'}
    aria-label="Resize the panels"
    aria-valuenow={Math.round(leftWidth * 100)}
    aria-valuemin={Math.round(effectiveMinLeft * 100)}
    aria-valuemax={Math.round(effectiveMaxLeft * 100)}
    tabindex="0"
  >
    <div class="col-resize-bar__line"></div>
  </div>

  <div
    class="panel-shell panel-detail min-h-0 min-w-0 overflow-hidden {stacked
      ? 'rounded-t-none rounded-b-xl'
      : 'rounded-xl'}"
    id={rightPanelID}
    inert={rightCollapsed}
    aria-hidden={rightCollapsed}
    style:flex={stacked
      ? `${rightCollapsed ? 0 : 1 - renderedLeftWidth} 1 0px`
      : `0 0 ${rightCollapsed ? 0 : rightSize}px`}
  >
    <PanelClip
      axis={stacked ? 'height' : 'width'}
      minimumSize={stacked
        ? flexSpacePx() * (1 - effectiveMaxLeft)
        : Math.max(rightSize, flexSpacePx() * (1 - effectiveMaxLeft))}
    >
      {@render rightPanel()}
    </PanelClip>
  </div>
</div>

<style lang="postcss">
  @reference "../../app.css";
  .panel-shell {
    @apply flex min-h-0 flex-col;
    transition:
      flex-basis var(--panel-motion-duration) var(--panel-motion-easing),
      flex-grow var(--panel-motion-duration) var(--panel-motion-easing),
      visibility 0s;
  }

  .panel-split {
    gap: var(--panel-split-flex-gap);
    transition: gap var(--panel-motion-duration) var(--panel-motion-easing);
  }

  .col-resize-bar {
    flex-basis: var(--resize-bar-hit-width);
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    transition:
      flex-basis var(--panel-motion-duration) var(--panel-motion-easing),
      visibility 0s;
  }

  .panel-split--collapsed {
    gap: 0;
  }

  .panel-split--collapsed .col-resize-bar {
    flex-basis: 0;
    visibility: hidden;
    transition-delay: 0s, var(--panel-motion-duration);
  }

  .panel-split--collapsed .panel-detail {
    visibility: hidden;
    transition-delay: 0s, 0s, var(--panel-motion-duration);
  }

  .panel-split--initializing,
  .panel-split--initializing .panel-shell,
  .panel-split--initializing .col-resize-bar,
  .panel-split--dragging,
  .panel-split--dragging .panel-shell,
  .panel-split--dragging .col-resize-bar {
    transition: none;
  }
</style>
