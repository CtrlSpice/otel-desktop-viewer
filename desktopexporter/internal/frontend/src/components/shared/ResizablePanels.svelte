<script lang="ts">
  import { onDestroy } from 'svelte'
  import { remToFraction } from '@/state/panel-width'
  import { startDrag, type DragHandle } from './utils/drag'

  /** Default split when no prop / storage; keep in sync with prop default below */
  const DEFAULT_LEFT_WIDTH = 0.7

  type Props = {
    leftPanel: any
    rightPanel: any
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
  }

  let {
    leftPanel,
    rightPanel,
    defaultLeftWidth = DEFAULT_LEFT_WIDTH,
    defaultRightRem,
    minLeftWidth = 0.3,
    minRightWidth = 0.2,
    minLeftPx,
    minRightPx,
    maxLeftPx,
    maxRightPx,
    storageKey,
    stackBreakpoint = 800,
  }: Props = $props()

  let leftWidth = $state(DEFAULT_LEFT_WIDTH)
  let appliedInitialDefault = $state(false)
  let isDragging = $state(false)

  let containerRef = $state<HTMLDivElement | null>(null)
  let dividerRef = $state<HTMLElement | null>(null)
  let containerWidth = $state(0)
  let containerHeight = $state(0)

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
    const divSize = dividerRef
      ? stacked
        ? dividerRef.offsetHeight
        : dividerRef.offsetWidth
      : 0
    return Math.max(1, raw - divSize - 2 * panelSplitGapPx())
  }

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
    if (storageKey) {
      let saved = localStorage.getItem(storageKey)
      if (saved) {
        let parsed = parseFloat(saved)
        // Clamped, not rejected: a split saved on a wide window used to
        // be discarded wholesale on a narrow one, silently reverting to
        // the default. The nearest legal split is what the person meant.
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
    const lo = effectiveMinLeft
    const hi = effectiveMaxLeft
    if (lo > hi) return
    if (leftWidth < lo) leftWidth = lo
    else if (leftWidth > hi) leftWidth = hi
  })

  function saveWidth() {
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
    drag = startDrag(e, {
      axis: stacked ? 'y' : 'x',
      onMove: delta => {
        leftWidth = Math.max(
          effectiveMinLeft,
          Math.min(effectiveMaxLeft, startWidth + delta / flexSpace)
        )
      },
      onEnd: () => {
        isDragging = false
        drag = null
        saveWidth()
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

  $effect(() => {
    if (!containerRef) return
    const ro = new ResizeObserver(entries => {
      for (const entry of entries) {
        containerWidth = entry.contentRect.width
        containerHeight = entry.contentRect.height
      }
    })
    ro.observe(containerRef)
    return () => ro.disconnect()
  })
</script>

{#if stacked}
  <div
    class="flex h-full w-full flex-col gap-[var(--panel-split-flex-gap)]"
    bind:this={containerRef}
  >
    <div
      class="panel-shell min-h-0 overflow-hidden rounded-t-xl rounded-b-none"
      style="flex: {leftWidth} 1 0px"
    >
      {@render leftPanel()}
    </div>

    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      bind:this={dividerRef}
      class="col-resize-bar col-resize-bar--row-in-flow"
      class:col-resize-bar--active={isDragging}
      onpointerdown={handlePointerDown}
      ondblclick={handleDoubleClick}
      onkeydown={handleKeydown}
      role="separator"
      aria-orientation="horizontal"
      aria-label="Resize the panels"
      aria-valuenow={Math.round(leftWidth * 100)}
      aria-valuemin={Math.round(effectiveMinLeft * 100)}
      aria-valuemax={Math.round(effectiveMaxLeft * 100)}
      tabindex="0"
    >
      <div class="col-resize-bar__line"></div>
    </div>

    <div
      class="panel-shell min-h-0 overflow-hidden rounded-t-none rounded-b-xl"
      style="flex: {1 - leftWidth} 1 0px"
    >
      {@render rightPanel()}
    </div>
  </div>
{:else}
  <div
    class="flex h-full w-full gap-[var(--panel-split-flex-gap)]"
    bind:this={containerRef}
  >
    <div
      class="panel-shell h-full min-w-0 overflow-hidden rounded-xl"
      style="flex: {leftWidth} 1 0px"
    >
      {@render leftPanel()}
    </div>

    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      bind:this={dividerRef}
      class="col-resize-bar col-resize-bar--in-flow"
      class:col-resize-bar--active={isDragging}
      onpointerdown={handlePointerDown}
      ondblclick={handleDoubleClick}
      onkeydown={handleKeydown}
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize the panels"
      aria-valuenow={Math.round(leftWidth * 100)}
      aria-valuemin={Math.round(effectiveMinLeft * 100)}
      aria-valuemax={Math.round(effectiveMaxLeft * 100)}
      tabindex="0"
    >
      <div class="col-resize-bar__line"></div>
    </div>

    <div
      class="panel-shell h-full min-w-0 overflow-hidden rounded-xl"
      style="flex: {1 - leftWidth} 1 0px"
    >
      {@render rightPanel()}
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";
  .panel-shell {
    @apply flex min-h-0 flex-col;
  }
</style>
