<script lang="ts">
  import type { Snippet } from 'svelte'
  import { setContext } from 'svelte'
  import VirtualList from '@humanspeak/svelte-virtual-list'
  import { ArrowRightIcon, ReloadIcon } from '@/icons'
  import ThemeToggle from '@/components/ThemeToggle.svelte'
  import DrawerNavTabs from '@/components/DrawerNavTabs.svelte'
  import {
    SIGNAL_DRAWER_CHROME_KEY,
    type SignalDrawerChrome,
  } from '@/contexts/signal-drawer-chrome.svelte'

  type Props<T> = {
    items: T[]
    selectedId: string | null
    drawerId: string
    label: string
    count: number
    storageKey: string
    itemSnippet: Snippet<[item: T, selected: boolean]>
    itemKey?: (item: T) => string
    onSelect?: (id: string) => void
    onRefresh?: () => void
    refreshPulse?: boolean
    /** Plain text for DaisyUI tooltip + screen reader when new data is pending */
    refreshAsideTip?: string
    drawerChrome?: Snippet
    drawerChromeToolbar?: Snippet
    drawerSearch?: Snippet
    footer?: Snippet
    children: Snippet
  }

  let {
    items,
    selectedId,
    drawerId,
    label,
    count,
    storageKey,
    itemSnippet,
    itemKey = (item: any) => item.id,
    onSelect,
    onRefresh,
    refreshPulse = false,
    refreshAsideTip = '',
    drawerChrome,
    drawerChromeToolbar,
    drawerSearch,
    footer,
    children,
  }: Props<any> = $props()

  let drawerOpen = $state(loadOpen())

  // When the drawer has no items (e.g. on Home), force-collapse it and hide
  // the open-toggle. We deliberately skip the localStorage write in that
  // state so an empty page can't clobber a real preference set on a page
  // that does have items.
  let isEmpty = $derived(items.length === 0)
  let effectivelyOpen = $derived(isEmpty ? false : drawerOpen)

  function loadOpen(): boolean {
    if (typeof localStorage === 'undefined') return true
    const v = localStorage.getItem(storageKey + ':open')
    return v === null ? true : v === 'true'
  }

  function handleToggleChange(e: Event) {
    if (isEmpty) return
    drawerOpen = (e.currentTarget as HTMLInputElement).checked
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(storageKey + ':open', String(drawerOpen))
    }
  }

  let drawerChromeContext = $state<SignalDrawerChrome>({
    closeForId: undefined,
  })
  setContext(SIGNAL_DRAWER_CHROME_KEY, drawerChromeContext)
  $effect(() => {
    drawerChromeContext.closeForId = effectivelyOpen ? drawerId : undefined
  })

  // --- auto-scroll the virtual list when the selection changes ---
  // Only fires when `selectedId` actually changes (not on items reshuffles),
  // so the user is free to scroll the list independently.
  type VirtualListRef = {
    scroll: (options: {
      index: number
      smoothScroll?: boolean
      shouldThrowOnBounds?: boolean
      align?: 'auto' | 'top' | 'bottom' | 'nearest'
    }) => Promise<void>
  }
  let vlistRef = $state<VirtualListRef | null>(null)
  let drawerBodyEl = $state<HTMLDivElement | null>(null)
  let lastScrolledSelection: string | null = null

  $effect(() => {
    if (!effectivelyOpen) {
      lastScrolledSelection = null
    }
  })

  // Pixels of breathing room required at top/bottom for an item to count as
  // "comfortably visible". If a partially-clipped row has at least this much
  // visible margin from the closest edge, we leave the viewport alone.
  const VISIBLE_MARGIN_PX = 24

  function isComfortablyVisible(idx: number): boolean {
    const viewport = drawerBodyEl?.querySelector<HTMLElement>(
      '.signal-drawer__vlist-viewport'
    )
    const row = viewport?.querySelector<HTMLElement>(
      `[data-original-index="${idx}"]`
    )
    if (!viewport || !row) return false
    const vRect = viewport.getBoundingClientRect()
    const rRect = row.getBoundingClientRect()
    return (
      rRect.top >= vRect.top + VISIBLE_MARGIN_PX &&
      rRect.bottom <= vRect.bottom - VISIBLE_MARGIN_PX
    )
  }

  $effect(() => {
    const id = selectedId
    if (!effectivelyOpen || !vlistRef || !id) return
    if (id === lastScrolledSelection) return
    const idx = items.findIndex(item => itemKey(item) === id)
    if (idx < 0) return
    lastScrolledSelection = id
    if (isComfortablyVisible(idx)) return
    void vlistRef.scroll({
      index: idx,
      align: 'auto',
      smoothScroll: true,
      shouldThrowOnBounds: false,
    })
  })
</script>

<div class="signal-drawer drawer drawer-open">
  <input
    id={drawerId}
    type="checkbox"
    class="drawer-toggle signal-drawer-toggle"
    checked={effectivelyOpen}
    disabled={isEmpty}
    onchange={handleToggleChange}
  />

  <div class="drawer-content min-h-0 min-w-0">
    {@render children()}
  </div>

  <div class="drawer-side is-drawer-close:overflow-visible">
    <div
      class="signal-drawer__panel flex h-full flex-col bg-base-100/95 is-drawer-close:w-14 is-drawer-open:w-96"
    >
      {#if !effectivelyOpen && !isEmpty}
        <!-- Collapsed: open-sidebar toggle pinned to the very top -->
        <div class="signal-drawer__open-toggle">
          <label
            for={drawerId}
            class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-right cursor-pointer"
            data-tip={label}
            aria-label="Open sidebar"
          >
            <ArrowRightIcon class="h-[17px] w-[17px]" aria-hidden="true" />
          </label>
        </div>
      {/if}

      {#if !effectivelyOpen}
        <!-- Collapsed: primary nav (icons) -->
        <div class="signal-drawer__nav signal-drawer__nav--collapsed">
          <DrawerNavTabs collapsed />
        </div>
      {/if}

      <!-- Collapsed: header controls (refresh, aside, theme toggle) -->
      {#if !effectivelyOpen}
        <div class="signal-drawer__collapsed-header">
          <div
            class="signal-drawer__header-controls--collapsed flex shrink-0 flex-col items-center gap-1.5"
          >
            {#if onRefresh}
              <div
                class="shrink-0 {refreshPulse && refreshAsideTip
                  ? 'tooltip tooltip-right tooltip-secondary'
                  : ''}"
                data-tip={refreshPulse && refreshAsideTip
                  ? refreshAsideTip
                  : undefined}
              >
                {#if refreshPulse && refreshAsideTip}
                  <div class="sr-only" aria-live="polite" aria-atomic="true">
                    {refreshAsideTip}
                  </div>
                {/if}
                <button
                  type="button"
                  class="signal-drawer__refresh drawer-header-btn drawer-header-btn--inactive"
                  class:signal-drawer__refresh--has-new-data={refreshPulse}
                  onclick={onRefresh}
                  aria-label={refreshPulse
                    ? `Refresh — ${refreshAsideTip}`
                    : 'Refresh'}
                >
                  {#if refreshPulse}
                    <span
                      class="signal-drawer__new-data-dot"
                      aria-hidden="true"
                    ></span>
                  {/if}
                  <ReloadIcon
                    class="relative z-[1] h-[17px] w-[17px] shrink-0"
                    aria-hidden="true"
                  />
                </button>
              </div>
            {/if}
            <ThemeToggle
              class="drawer-header-btn drawer-header-btn--inactive"
            />
          </div>
        </div>
      {/if}

      <!-- Expanded: unified header panel (tabs + chrome + search + toolbar) -->
      {#if effectivelyOpen}
        <div class="signal-drawer__header is-drawer-close:hidden">
          <div class="signal-drawer__chrome-stack">
            <DrawerNavTabs />
            {#if drawerChrome}
              <div class="signal-drawer__toolbar-slot">
                {@render drawerChrome()}
              </div>
            {/if}
          </div>

          {#if drawerSearch || drawerChromeToolbar}
            <div class="signal-drawer__search-stack">
              {#if drawerSearch}
                <div class="signal-drawer__search">
                  {@render drawerSearch()}
                </div>
              {/if}
              {#if drawerChromeToolbar}
                <div class="signal-drawer__chrome-toolbar">
                  {@render drawerChromeToolbar()}
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}

      <!-- Collapsed: count badge -->
      {#if !isEmpty}
        <div class="signal-drawer__rail-count is-drawer-open:hidden">
          {count}
        </div>
      {/if}

      <!-- Expanded: list -->
      <div
        class="signal-drawer__body is-drawer-close:hidden"
        bind:this={drawerBodyEl}
      >
        <VirtualList
          bind:this={vlistRef}
          {items}
          defaultEstimatedItemHeight={72}
          bufferSize={10}
          containerClass="signal-drawer__vlist"
          viewportClass="signal-drawer__vlist-viewport"
          itemsClass="signal-drawer__vlist-items"
        >
          {#snippet renderItem(item)}
            {@render itemSnippet(item, selectedId === itemKey(item))}
          {/snippet}
        </VirtualList>
      </div>

      <!-- Expanded: footer -->
      {#if footer}
        <div class="signal-drawer__footer is-drawer-close:hidden">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
</div>

<style lang="postcss">
  @reference "../app.css";

  .signal-drawer {
    @apply min-h-0 flex-1 overflow-hidden;
  }

  .signal-drawer .drawer-content {
    @apply flex flex-col;
  }

  .signal-drawer :global(.drawer-side) {
    @apply h-full overflow-hidden;
    min-height: 0;
  }

  .signal-drawer__panel {
    @apply transition-[width] duration-200;
    border-right: 1px solid
      color-mix(in oklab, var(--color-base-300) 70%, transparent);
  }

  /* ── Collapsed: open-sidebar toggle pinned to the top ── */
  .signal-drawer__open-toggle {
    @apply flex shrink-0 items-center justify-center px-1.5 pt-2 pb-1.5;
  }

  /* ── Collapsed: icon rail ── */
  .signal-drawer__nav--collapsed {
    @apply shrink-0 px-1.5 pt-2 pb-2;
  }

  /* ── Expanded: unified header panel ── */
  .signal-drawer__header {
    @apply flex w-full min-w-0 shrink-0 flex-col gap-1 px-2 py-1.5
      border-b border-base-300;
    background-image: linear-gradient(
      to bottom,
      color-mix(in oklab, var(--color-base-200) 80%, transparent),
      color-mix(in oklab, var(--color-base-200) 60%, transparent)
    );
    box-shadow:
      inset 0 1px 0
        color-mix(in oklab, var(--color-base-100) 60%, transparent),
      inset 0 -1px 0
        color-mix(in oklab, var(--color-base-300) 30%, transparent);
  }

  .signal-drawer__chrome-stack {
    @apply flex shrink-0 items-center gap-1;
  }

  .signal-drawer__chrome-stack :global(.drawer-nav-tabs) {
    @apply flex-1 min-w-0;
  }

  .signal-drawer__toolbar-slot {
    @apply shrink-0;
  }

  /* ── Collapsed: header controls ── */
  .signal-drawer__collapsed-header {
    @apply flex w-full min-w-0 shrink-0 flex-col items-center justify-center gap-1.5 px-2 py-2;
  }

  /* Refresh + new-data indicator */
  .signal-drawer__refresh {
    @apply relative rounded-lg;
  }

  .signal-drawer__new-data-dot {
    @apply pointer-events-none absolute bottom-0.5 right-0.5 z-[2] size-2 rounded-full bg-secondary shadow-sm ring-2 ring-base-100/90;
  }

  @keyframes signal-drawer-new-data-dot-pulse {
    0%,
    100% {
      box-shadow:
        0 0 0 1px color-mix(in oklab, var(--color-secondary) 18%, transparent),
        0 0 10px color-mix(in oklab, var(--color-secondary) 12%, transparent);
    }
    50% {
      box-shadow:
        0 0 0 1px color-mix(in oklab, var(--color-secondary) 38%, transparent),
        0 0 22px color-mix(in oklab, var(--color-secondary) 28%, transparent);
    }
  }

  .signal-drawer__refresh.signal-drawer__refresh--has-new-data:not(
      :hover
    ):not(:focus-visible)
    .signal-drawer__new-data-dot {
    animation: signal-drawer-new-data-dot-pulse 2.8s ease-in-out infinite;
  }

  @media (prefers-reduced-motion: reduce) {
    .signal-drawer__refresh.signal-drawer__refresh--has-new-data
      .signal-drawer__new-data-dot {
      animation: none !important;
    }
  }

  /* ── Search + list-controls stack ── */
  .signal-drawer__search-stack {
    @apply flex min-w-0 w-full shrink-0 flex-col gap-1;
  }

  .signal-drawer__search {
    @apply min-w-0 w-full shrink-0;
  }

  /* ── Toolbar row (sort · time · refresh) ── */
  .signal-drawer__chrome-toolbar {
    @apply min-w-0 w-full shrink-0;
  }

  .signal-drawer__chrome-toolbar :global(.drawer-search-panel) {
    @apply gap-0;
  }

  /* ── Rail count badge (collapsed) ── */
  .signal-drawer__rail-count {
    @apply mt-2 self-center rounded-md bg-base-200/70 px-1.5 py-0.5 text-[0.6rem] font-semibold tabular-nums text-base-content/60;
  }

  /* ── Body (list) ── */
  .signal-drawer__body {
    @apply flex-1 min-h-0 overflow-hidden;
  }

  .signal-drawer__body :global(.signal-drawer__vlist) {
    @apply relative h-full w-full overflow-hidden;
  }

  .signal-drawer__body :global(.signal-drawer__vlist-viewport) {
    @apply absolute inset-0 overflow-y-scroll;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: thin;
  }

  .signal-drawer__body :global(.signal-drawer__vlist-items) {
    @apply absolute left-0 top-0 w-full;
  }

  /* Vertical rhythm between cards (padding counts toward measured row height; margin does not). */
  .signal-drawer__body :global(.signal-drawer__vlist-items > div) {
    @apply pb-2;
  }

  /* ── Footer ──
     Pinned to --app-footer-height (defined in app.css) so the
     drawer's bottom strip aligns pixel-for-pixel with the page
     footer in PageLayout. Vertical padding is replaced by
     min-height + items-center so the strip doesn't collapse around
     small controls (btn-xs) or grow with larger ones (btn-sm).
     The single direct child stretches to fill the row so consumers
     don't have to remember to add w-full themselves. */
  .signal-drawer__footer {
    @apply flex shrink-0 items-center border-t border-base-300/40 px-3;
    min-height: var(--app-footer-height);
  }

  .signal-drawer__footer > :global(*) {
    @apply min-w-0 flex-1;
  }
</style>
