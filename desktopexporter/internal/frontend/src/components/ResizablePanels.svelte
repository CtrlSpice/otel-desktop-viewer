<script lang="ts">
  /** Default split when no prop / storage; keep in sync with prop default below */
  const DEFAULT_LEFT_WIDTH = 0.7;

  type Props = {
    leftPanel: any;
    rightPanel: any;
    defaultLeftWidth?: number;
    minLeftWidth?: number;
    minRightWidth?: number;
    storageKey?: string;
    stackBreakpoint?: number;
  };

  let {
    leftPanel,
    rightPanel,
    defaultLeftWidth = DEFAULT_LEFT_WIDTH,
    minLeftWidth = 0.3,
    minRightWidth = 0.2,
    storageKey,
    stackBreakpoint = 700,
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
  let dividerRef = $state<HTMLDivElement | null>(null);
  let containerWidth = $state(0);

  /** Matches CSS `gap` on the flex container (`--panel-split-flex-gap`). */
  function panelSplitGapPx(): number {
    if (!containerRef) return 8;
    const s = getComputedStyle(containerRef);
    const raw = stacked ? s.rowGap : s.columnGap;
    const px = parseFloat(raw);
    return Number.isFinite(px) ? px : 8;
  }

  let stacked = $derived(containerWidth > 0 && containerWidth < stackBreakpoint);

  $effect(() => {
    if (storageKey) {
      let saved = localStorage.getItem(storageKey);
      if (saved) {
        let parsed = parseFloat(saved);
        if (
          !isNaN(parsed) &&
          parsed >= minLeftWidth &&
          parsed <= 1 - minRightWidth
        ) {
          leftWidth = parsed;
        }
      }
    }
  });

  function saveWidth() {
    if (storageKey) {
      localStorage.setItem(storageKey, leftWidth.toString());
    }
  }

  let dragStartPos = 0;
  let dragStartWidth = 0;

  function handlePointerDown(e: PointerEvent) {
    e.preventDefault();
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    isDragging = true;
    dragStartPos = stacked ? e.clientY : e.clientX;
    dragStartWidth = leftWidth;
    document.body.style.cursor = stacked ? 'row-resize' : 'col-resize';
    document.body.style.userSelect = 'none';
  }

  function handlePointerMove(e: PointerEvent) {
    if (!isDragging || !containerRef || !dividerRef) return;
    const g = panelSplitGapPx();
    const currentPos = stacked ? e.clientY : e.clientX;
    const deltaPx = currentPos - dragStartPos;
    if (stacked) {
      const divH = dividerRef.offsetHeight;
      const flexSpace = Math.max(1, containerRef.getBoundingClientRect().height - divH - 2 * g);
      leftWidth = Math.max(minLeftWidth, Math.min(1 - minRightWidth, dragStartWidth + deltaPx / flexSpace));
    } else {
      const divW = dividerRef.offsetWidth;
      const flexSpace = Math.max(1, containerRef.getBoundingClientRect().width - divW - 2 * g);
      leftWidth = Math.max(minLeftWidth, Math.min(1 - minRightWidth, dragStartWidth + deltaPx / flexSpace));
    }
  }

  function handlePointerUp(e: PointerEvent) {
    if (!isDragging) return;
    const target = e.currentTarget as HTMLElement;
    target.releasePointerCapture(e.pointerId);
    isDragging = false;
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
    saveWidth();
  }

  function handleDoubleClick() {
    leftWidth = defaultLeftWidth;
    saveWidth();
  }

  function handleKeydown(e: KeyboardEvent) {
    const step = e.shiftKey ? 0.05 : 0.01;
    if (stacked) {
      if (e.key === 'ArrowUp' && leftWidth > minLeftWidth) {
        e.preventDefault();
        leftWidth = Math.max(minLeftWidth, leftWidth - step);
        saveWidth();
      } else if (e.key === 'ArrowDown' && leftWidth < 1 - minRightWidth) {
        e.preventDefault();
        leftWidth = Math.min(1 - minRightWidth, leftWidth + step);
        saveWidth();
      }
    } else {
      if (e.key === 'ArrowLeft' && leftWidth > minLeftWidth) {
        e.preventDefault();
        leftWidth = Math.max(minLeftWidth, leftWidth - step);
        saveWidth();
      } else if (e.key === 'ArrowRight' && leftWidth < 1 - minRightWidth) {
        e.preventDefault();
        leftWidth = Math.min(1 - minRightWidth, leftWidth + step);
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
    class="flex h-full w-full flex-col gap-[var(--panel-split-flex-gap)]"
    bind:this={containerRef}
  >
    <div
      class="panel-shell min-h-0 overflow-hidden rounded-xl"
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
      onpointermove={handlePointerMove}
      onpointerup={handlePointerUp}
      ondblclick={handleDoubleClick}
      onkeydown={handleKeydown}
      role="separator"
      aria-orientation="horizontal"
      aria-valuenow={Math.round(leftWidth * 100)}
      aria-valuemin={Math.round(minLeftWidth * 100)}
      aria-valuemax={Math.round((1 - minRightWidth) * 100)}
      tabindex="0"
    >
      <div class="col-resize-bar__line"></div>
    </div>

    <div
      class="panel-shell min-h-0 overflow-hidden rounded-xl"
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
      aria-valuemin={Math.round(minLeftWidth * 100)}
      aria-valuemax={Math.round((1 - minRightWidth) * 100)}
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
