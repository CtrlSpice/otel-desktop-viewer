<script lang="ts">
  import { onDestroy, setContext } from 'svelte'
  import {
    PANEL_SPLIT_RESIZE_KEY,
    type PanelSplitResizeContext,
  } from '@/contexts/panel-split-resize-context.svelte'

  /** Default split when no prop / storage; keep in sync with prop default below */
  const DEFAULT_LEFT_WIDTH = 0.7;

  type Props = {
    leftPanel: any;
    rightPanel: any;
    /** Stacked layout only: use the bottom panel header as the resize
     *  handle instead of a separate divider strip. */
    stackedResizeHandle?: 'bar' | 'panel-header'
    defaultLeftWidth?: number;
    /** Minimum left fraction of the container (0..1). */
    minLeftWidth?: number;
    /** Minimum right fraction of the container (0..1). */
    minRightWidth?: number;
    /** Optional absolute pixel floor for the left pane. When set,
     *  the drag clamps to MAX(fraction floor, pixel floor). Lets
     *  callers guarantee enough room for fixed-size chrome (e.g.
     *  a tab strip) regardless of viewport width. */
    minLeftPx?: number;
    /** Optional absolute pixel floor for the right pane. */
    minRightPx?: number;
    storageKey?: string;
    stackBreakpoint?: number;
  };

  let {
    leftPanel,
    rightPanel,
    stackedResizeHandle = 'bar',
    defaultLeftWidth = DEFAULT_LEFT_WIDTH,
    minLeftWidth = 0.3,
    minRightWidth = 0.2,
    minLeftPx,
    minRightPx,
    storageKey,
    stackBreakpoint = 800,
  }: Props = $props();

  let leftWidth = $state(DEFAULT_LEFT_WIDTH);
  let appliedInitialDefault = $state(false);
  let isDragging = $state(false);

  $effect.pre(() => {
    if (appliedInitialDefault) return;
    leftWidth = defaultLeftWidth;
    appliedInitialDefault = true;
  });
  let containerRef = $state<HTMLDivElement | null>(null);
  let dividerRef = $state<HTMLElement | null>(null);
  let containerWidth = $state(0);

  /** Matches CSS `gap` on the flex container (`--panel-split-flex-gap`). */
  function panelSplitGapPx(): number {
    if (!containerRef) return 8;
    const s = getComputedStyle(containerRef);
    const raw = stacked ? s.rowGap : s.columnGap;
    const px = parseFloat(raw);
    return Number.isFinite(px) ? px : 8;
  }

  let stacked = $derived(containerWidth > 0 && containerWidth < stackBreakpoint)
  let usePanelHeaderResize = $derived(
    stacked && stackedResizeHandle === 'panel-header'
  )
  let stackedGapClass = $derived(
    usePanelHeaderResize ? 'gap-0' : 'gap-[var(--panel-split-flex-gap)]'
  )

  const panelSplitResizeCtx: PanelSplitResizeContext = {
    registerHandle(el) {
      dividerRef = el
    },
    onPointerDown: handlePointerDown,
    onPointerMove: handlePointerMove,
    onPointerUp: handlePointerUp,
    onDoubleClick: handleDoubleClick,
    onKeydown: handleKeydown,
    get isDragging() {
      return isDragging
    },
    get ariaNow() {
      return Math.round(leftWidth * 100)
    },
    get ariaMin() {
      return Math.round(effectiveMinLeft * 100)
    },
    get ariaMax() {
      return Math.round((1 - effectiveMinRight) * 100)
    },
  }

  if (stackedResizeHandle === 'panel-header') {
    setContext(PANEL_SPLIT_RESIZE_KEY, panelSplitResizeCtx)
  }

  /* Pixel floors → fractions of the current container (horizontal
     mode only). The drag clamp uses MAX(fraction floor, pixel-derived
     floor) so callers can guarantee chrome fits without breaking the
     existing fraction API. In stacked (vertical) mode the pixel
     floors are ignored — panes go full-width and a row-resize
     controls height instead.

     Graceful fallback: when the container is too narrow to honor
     both pixel floors (minLeftPx + minRightPx > containerWidth), we
     prefer to keep the right pane's pixel floor and shrink the left
     to whatever's left. The right pane houses the inspector tab
     strip, which has a fixed minimum width below which it visibly
     truncates — main views (waterfall, chart) tolerate narrowness
     better. If even that's impossible (right floor alone exceeds the
     container), we drop both pixel floors and fall back to the
     fraction bounds. */
  let effectiveMins = $derived.by<{ left: number; right: number }>(() => {
    if (stacked || containerWidth <= 0) {
      return { left: minLeftWidth, right: minRightWidth };
    }

    const leftPxFrac = minLeftPx ? minLeftPx / containerWidth : 0;
    const rightPxFrac = minRightPx ? minRightPx / containerWidth : 0;
    let left = Math.max(minLeftWidth, leftPxFrac);
    let right = Math.max(minRightWidth, rightPxFrac);

    if (left + right <= 1) return { left, right };

    /* Try keeping the right (detail) pixel floor and shrinking left
       to the remaining space, but never below its fraction floor. */
    if (right <= 1 - minLeftWidth) {
      return { left: Math.max(minLeftWidth, 1 - right), right };
    }

    /* Right floor alone is too big — drop pixel floors entirely. */
    return { left: minLeftWidth, right: minRightWidth };
  });
  let effectiveMinLeft = $derived(effectiveMins.left);
  let effectiveMinRight = $derived(effectiveMins.right);

  $effect(() => {
    if (storageKey) {
      let saved = localStorage.getItem(storageKey);
      if (saved) {
        let parsed = parseFloat(saved);
        if (
          !isNaN(parsed) &&
          parsed >= effectiveMinLeft &&
          parsed <= 1 - effectiveMinRight
        ) {
          leftWidth = parsed;
        }
      }
    }
  });

  /* Re-clamp the current width whenever the effective minimums move.
     This catches the viewport-shrink case: if the user makes the
     window narrow enough that the current split would put one pane
     below its pixel floor, snap it back to the floor. */
  $effect(() => {
    const lo = effectiveMinLeft;
    const hi = 1 - effectiveMinRight;
    if (lo > hi) return;
    if (leftWidth < lo) leftWidth = lo;
    else if (leftWidth > hi) leftWidth = hi;
  });

  function saveWidth() {
    if (storageKey) {
      localStorage.setItem(storageKey, leftWidth.toString());
    }
  }

  let dragStartPos = 0;
  let dragStartWidth = 0;
  let dragFlexSpace = 1;
  let activePointerId: number | null = null;
  let captureEl: HTMLElement | null = null;

  function onWindowPointerMove(e: PointerEvent) {
    if (!isDragging || e.pointerId !== activePointerId) return;
    const currentPos = stacked ? e.clientY : e.clientX;
    const deltaPx = currentPos - dragStartPos;
    leftWidth = Math.max(
      effectiveMinLeft,
      Math.min(1 - effectiveMinRight, dragStartWidth + deltaPx / dragFlexSpace)
    );
  }

  function endDrag() {
    if (!isDragging) return;
    const pointerId = activePointerId;
    const el = captureEl;
    isDragging = false;
    activePointerId = null;
    captureEl = null;
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
    window.removeEventListener('pointermove', onWindowPointerMove);
    window.removeEventListener('pointerup', onWindowPointerEnd);
    window.removeEventListener('pointercancel', onWindowPointerEnd);
    if (el && pointerId !== null) {
      try {
        el.releasePointerCapture(pointerId);
      } catch {
        /* capture already released */
      }
    }
    saveWidth();
  }

  function onWindowPointerEnd(e: PointerEvent) {
    if (!isDragging || e.pointerId !== activePointerId) return;
    endDrag();
  }

  function handlePointerDown(e: PointerEvent) {
    if (isDragging) return;
    e.preventDefault();
    const target = e.currentTarget as HTMLElement;
    captureEl = target;
    activePointerId = e.pointerId;
    try {
      target.setPointerCapture(e.pointerId);
    } catch {
      /* ignore — window listeners still end the drag */
    }
    isDragging = true;
    dragStartPos = stacked ? e.clientY : e.clientX;
    dragStartWidth = leftWidth;

    if (containerRef) {
      const rect = containerRef.getBoundingClientRect();
      if (stacked && usePanelHeaderResize) {
        dragFlexSpace = Math.max(1, rect.height);
      } else if (dividerRef) {
        const g = panelSplitGapPx();
        const divSize = stacked ? dividerRef.offsetHeight : dividerRef.offsetWidth;
        dragFlexSpace = Math.max(
          1,
          (stacked ? rect.height : rect.width) - divSize - 2 * g
        );
      }
    }

    document.body.style.cursor = stacked ? 'row-resize' : 'col-resize';
    document.body.style.userSelect = 'none';
    window.addEventListener('pointermove', onWindowPointerMove);
    window.addEventListener('pointerup', onWindowPointerEnd);
    window.addEventListener('pointercancel', onWindowPointerEnd);
  }

  function handlePointerMove(e: PointerEvent) {
    onWindowPointerMove(e);
  }

  function handlePointerUp(e: PointerEvent) {
    onWindowPointerEnd(e);
  }

  onDestroy(endDrag);

  function handleDoubleClick() {
    leftWidth = defaultLeftWidth;
    saveWidth();
  }

  function handleKeydown(e: KeyboardEvent) {
    const step = e.shiftKey ? 0.05 : 0.01;
    const lo = effectiveMinLeft;
    const hi = 1 - effectiveMinRight;
    if (stacked) {
      if (e.key === 'ArrowUp' && leftWidth > lo) {
        e.preventDefault();
        leftWidth = Math.max(lo, leftWidth - step);
        saveWidth();
      } else if (e.key === 'ArrowDown' && leftWidth < hi) {
        e.preventDefault();
        leftWidth = Math.min(hi, leftWidth + step);
        saveWidth();
      }
    } else {
      if (e.key === 'ArrowLeft' && leftWidth > lo) {
        e.preventDefault();
        leftWidth = Math.max(lo, leftWidth - step);
        saveWidth();
      } else if (e.key === 'ArrowRight' && leftWidth < hi) {
        e.preventDefault();
        leftWidth = Math.min(hi, leftWidth + step);
        saveWidth();
      }
    }
  }

  $effect(() => {
    if (!containerRef) return;
    const ro = new ResizeObserver(entries => {
      for (const entry of entries) {
        containerWidth = entry.contentRect.width;
      }
    });
    ro.observe(containerRef);
    return () => ro.disconnect();
  });
</script>

{#if stacked}
  <div
    class="flex h-full w-full flex-col {stackedGapClass}"
    bind:this={containerRef}
  >
    <div
      class="panel-shell min-h-0 overflow-hidden rounded-t-xl rounded-b-none"
      style="flex: {leftWidth} 1 0px"
    >
      {@render leftPanel()}
    </div>

    {#if !usePanelHeaderResize}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <div
        bind:this={dividerRef}
        class="col-resize-bar col-resize-bar--row-in-flow"
        class:col-resize-bar--active={isDragging}
        onpointerdown={handlePointerDown}
        onpointermove={handlePointerMove}
        onpointerup={handlePointerUp}
        ondblclick={handleDoubleClick}
        onkeydown={handleKeydown}
        role="separator"
        aria-orientation="horizontal"
        aria-valuenow={Math.round(leftWidth * 100)}
        aria-valuemin={Math.round(effectiveMinLeft * 100)}
        aria-valuemax={Math.round((1 - effectiveMinRight) * 100)}
        tabindex="0"
      >
        <div class="col-resize-bar__line"></div>
      </div>
    {/if}

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
      onpointermove={handlePointerMove}
      onpointerup={handlePointerUp}
      ondblclick={handleDoubleClick}
      onkeydown={handleKeydown}
      role="separator"
      aria-orientation="vertical"
      aria-valuenow={Math.round(leftWidth * 100)}
      aria-valuemin={Math.round(effectiveMinLeft * 100)}
      aria-valuemax={Math.round((1 - effectiveMinRight) * 100)}
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
  @reference "../app.css";
  .panel-shell {
    @apply min-h-0;
  }
</style>
