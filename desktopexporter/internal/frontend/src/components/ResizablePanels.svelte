<script lang="ts">
  type Props = {
    leftPanel: any;
    rightPanel: any;
    defaultLeftWidth?: number; // 0-1, percentage
    minLeftWidth?: number; // 0-1, minimum percentage
    minRightWidth?: number; // 0-1, minimum percentage
    storageKey?: string; // localStorage key for persistence
  };

  let {
    leftPanel,
    rightPanel,
    defaultLeftWidth = 0.6,
    minLeftWidth = 0.2,
    minRightWidth = 0.3,
    storageKey,
  }: Props = $props();

  let leftWidth = $state(defaultLeftWidth);
  let isDragging = $state(false);
  let containerRef: HTMLDivElement | null = null;

  // Load saved width from localStorage
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

  // Save width to localStorage
  function saveWidth() {
    if (storageKey) {
      localStorage.setItem(storageKey, leftWidth.toString());
    }
  }

  function handleMouseDown(e: MouseEvent) {
    e.preventDefault();
    isDragging = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  }

  function handleMouseMove(e: MouseEvent) {
    if (!isDragging || !containerRef) return;

    let rect = containerRef.getBoundingClientRect();
    let newLeftWidth = (e.clientX - rect.left) / rect.width;

    // Enforce minimum widths
    newLeftWidth = Math.max(
      minLeftWidth,
      Math.min(1 - minRightWidth, newLeftWidth)
    );

    leftWidth = newLeftWidth;
  }

  function handleMouseUp() {
    if (isDragging) {
      isDragging = false;
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      saveWidth();
    }
  }

  // Handle double-click to reset
  function handleDoubleClick() {
    leftWidth = defaultLeftWidth;
    saveWidth();
  }

  // Cleanup on unmount
  $effect(() => {
    if (typeof window !== 'undefined') {
      window.addEventListener('mousemove', handleMouseMove);
      window.addEventListener('mouseup', handleMouseUp);

      return () => {
        window.removeEventListener('mousemove', handleMouseMove);
        window.removeEventListener('mouseup', handleMouseUp);
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
      };
    }
  });
</script>

<div class="flex w-full h-full gap-2" bind:this={containerRef}>
  <!-- Left Panel -->
  <div style="width: {leftWidth * 100}%; flex-shrink: 0;">
    {@render leftPanel()}
  </div>

  <!-- Resize Handle -->
  <div
    class="resize-handle"
    class:active={isDragging}
    onmousedown={handleMouseDown}
    ondblclick={handleDoubleClick}
    role="button"
    aria-label="Resize panels"
    tabindex="0"
    onkeydown={e => {
      if (e.key === 'ArrowLeft' && leftWidth > minLeftWidth) {
        leftWidth = Math.max(minLeftWidth, leftWidth - 0.01);
        saveWidth();
      } else if (e.key === 'ArrowRight' && leftWidth < 1 - minRightWidth) {
        leftWidth = Math.min(1 - minRightWidth, leftWidth + 0.01);
        saveWidth();
      }
    }}
  >
    <div class="resize-handle-grip"></div>
  </div>

  <!-- Right Panel -->
  <div style="width: {(1 - leftWidth) * 100}%; flex-shrink: 0;">
    {@render rightPanel()}
  </div>
</div>

<style>
  .resize-handle {
    @apply w-1 h-full bg-base-300 hover:bg-base-content/20 transition-colors cursor-col-resize flex items-center justify-center relative;
    @apply select-none;
    flex-shrink: 0;
  }

  .resize-handle:hover {
    @apply bg-base-content/30;
  }

  .resize-handle.active {
    @apply bg-primary/40;
  }

  .resize-handle:focus {
    @apply outline-none ring-2 ring-primary ring-offset-2 ring-offset-base-100;
  }

  .resize-handle-grip {
    @apply w-0.5 h-8 bg-base-content/40 rounded-full;
  }

  .resize-handle:hover .resize-handle-grip {
    @apply bg-base-content/60;
  }

  .resize-handle.active .resize-handle-grip {
    @apply bg-primary-content;
  }
</style>
