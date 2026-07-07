<script module lang="ts">
  // Drawer open preference is persisted in localStorage, but each
  // signal route mounts its own SignalListDrawer. Without this
  // module-level cache, the panel width transition runs on every
  // navigation even when open/closed did not change.
  let lastOpen: boolean | undefined

  /** Skip the width tween when remounting with the same open preference. */
  function shouldSkipDrawerWidthTransition(open: boolean): boolean {
    const skip = lastOpen !== undefined && lastOpen === open
    lastOpen = open
    return skip
  }

  function syncDrawerOpenPreference(open: boolean): void {
    lastOpen = open
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte'
  import { onMount } from 'svelte'
  import VirtualList from '@humanspeak/svelte-virtual-list'
  import {
    ArrowRightIcon,
    ReloadIcon,
    BarChartHorizontalIcon,
    ChartHistogramIcon,
    LogIcon,
    HomeIcon,
  } from '@/icons'
  import ThemeToggle from '@/components/shared/ThemeToggle.svelte'
  import DrawerNavTabs from '@/components/shared/Drawer/DrawerNavTabs.svelte'
  import { NAV_ITEMS, isNavItemActive } from '@/components/shared/Drawer/DrawerNavTabs.svelte'
  import DateTimeFilter from '@/components/shared/Toolbar/DateTimeFilter.svelte'
  import PaneHeader, { type PaneTab } from '@/components/shared/PaneHeader.svelte'
  import { router } from 'tinro5'
  import { navigateToSignal, type SignalName } from '@/utils/url-state'
  import { useRoute } from '@/state/route.svelte'

  type Props<T> = {
    items: T[]
    selectedId: string | null
    drawerId: string
    label: string
    count: number
    itemSnippet: Snippet<[item: T, selected: boolean]>
    itemKey?: (item: T) => string
    onSelect?: (id: string) => void
    onRefresh?: () => void
    refreshPulse?: boolean
    /** Plain text for DaisyUI tooltip + screen reader when new data is pending */
    refreshAsideTip?: string
    /** When true, an empty list does not force-collapse the drawer (initial fetch). */
    loading?: boolean
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
    itemSnippet,
    itemKey = (item: any) => item.id,
    onSelect,
    onRefresh,
    refreshPulse = false,
    refreshAsideTip = '',
    loading = false,
    drawerChromeToolbar,
    drawerSearch,
    footer,
    children,
  }: Props<any> = $props()

  /*
   * Drawer open/closed is a single global preference shared by every
   * signal page. Each route mounts its own SignalListDrawer instance,
   * so they don't share in-memory state -- but they all read/write the
   * same localStorage key on mount/toggle, which gives "opened on
   * Traces => still opened on Logs" behavior with no cross-component
   * plumbing.
   */
  const DRAWER_OPEN_KEY = 'signal-drawer:open'

  function loadDrawerOpen(): boolean {
    if (typeof localStorage === 'undefined') return true
    const v = localStorage.getItem(DRAWER_OPEN_KEY)
    return v === null ? true : v === 'true'
  }

  const initialDrawerOpen = loadDrawerOpen()
  let drawerOpen = $state(initialDrawerOpen)
  // Suppress width tween when remounting across signal routes (same preference).
  let skipWidthTransition = $state(
    shouldSkipDrawerWidthTransition(initialDrawerOpen)
  )

  onMount(() => {
    requestAnimationFrame(() => {
      skipWidthTransition = false
    })
  })

  // Force-collapse only when the list is empty after load — not while the
  // initial fetch is in flight (otherwise every signal navigation briefly
  // collapses then re-opens). Toggle stays disabled when truly empty.
  let isEmpty = $derived(items.length === 0 && !loading)
  let effectivelyOpen = $derived(isEmpty ? false : drawerOpen)

  function handleToggleChange(e: Event) {
    if (isEmpty) return
    skipWidthTransition = false
    drawerOpen = (e.currentTarget as HTMLInputElement).checked
    syncDrawerOpenPreference(drawerOpen)
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(DRAWER_OPEN_KEY, String(drawerOpen))
    }
  }

  const route = useRoute()
  let activeNavId = $derived(
    NAV_ITEMS.find(n => isNavItemActive(n.id, route.path))?.id ?? NAV_ITEMS[0].id
  )

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
      class="signal-drawer__panel flex h-full flex-col is-drawer-close:w-14 is-drawer-open:w-[28rem] is-drawer-close:overflow-hidden is-drawer-close:bg-base-300 is-drawer-open:bg-base-200"
      class:signal-drawer__panel--instant={skipWidthTransition}
    >
      {#if !effectivelyOpen}
        <div class="signal-drawer__collapsed-rail">
          <div class="signal-drawer__collapsed-group">
          {#if isEmpty}
            <span
              class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-right"
              data-tip="Send data to populate this drawer"
              aria-disabled="true"
            >
              <ArrowRightIcon
                class="h-[17px] w-[17px] opacity-40"
                aria-hidden="true"
              />
            </span>
          {:else}
            <label
              for={drawerId}
              class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-right cursor-pointer"
              data-tip={label}
              aria-label="Open sidebar"
            >
              <ArrowRightIcon
                class="h-[17px] w-[17px] animate-[spin-half_200ms_ease-out]"
                aria-hidden="true"
              />
            </label>
          {/if}
          </div>

          <div class="separator w-8" aria-hidden="true"></div>

          <div class="signal-drawer__collapsed-group">
            <DrawerNavTabs collapsed />
          </div>

          <div class="separator w-8" aria-hidden="true"></div>
          <div class="signal-drawer__collapsed-group">
            <DateTimeFilter
              popoverAnchor="outward"
              class="drawer-header-btn drawer-header-btn--inactive shrink-0"
            />
            {#if onRefresh}
              <button
                type="button"
              class="signal-drawer__refresh drawer-header-btn drawer-header-btn--inactive {refreshPulse &&
              refreshAsideTip
                ? 'tooltip tooltip-right tooltip-secondary'
                : ''}"
              data-tip={refreshPulse && refreshAsideTip
                ? refreshAsideTip
                : undefined}
              class:signal-drawer__refresh--has-new-data={refreshPulse}
              onclick={onRefresh}
              aria-label={refreshPulse
                ? `Refresh — ${refreshAsideTip}`
                : 'Refresh'}
            >
              {#if refreshPulse && refreshAsideTip}
                <div class="sr-only" aria-live="polite" aria-atomic="true">
                  {refreshAsideTip}
                </div>
              {/if}
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
            {/if}
            <ThemeToggle
              class="drawer-header-btn drawer-header-btn--inactive"
            />
          </div>
        </div>
      {/if}

      <!-- Expanded: unified header panel (tabs + chrome + search + toolbar) -->
      {#if effectivelyOpen}
        {#snippet tracesIcon()}<BarChartHorizontalIcon class="h-[15px] w-[15px] shrink-0" />{/snippet}
        {#snippet metricsIcon()}<ChartHistogramIcon class="h-[15px] w-[15px] shrink-0" />{/snippet}
        {#snippet logsIcon()}<LogIcon class="h-[15px] w-[15px] shrink-0" />{/snippet}
        {@const navTabs: PaneTab[] = [
          { id: 'traces', label: 'Traces', icon: tracesIcon },
          { id: 'metrics', label: 'Metrics', icon: metricsIcon },
          { id: 'logs', label: 'Logs', icon: logsIcon },
        ]}
        <div class="signal-drawer__header is-drawer-close:hidden">
          <PaneHeader
            mode="tabs"
            tabs={navTabs}
            activeId={activeNavId}
            onSelect={(id) => {
              const item = NAV_ITEMS.find(n => n.id === id)
              // Switching signal is navigational: push (back returns to prior).
              if (item) navigateToSignal(item.id as SignalName, { replace: false })
            }}
            rounded={false}
            ariaLabel="Primary"
          >
            {#snippet right()}
              <button
                type="button"
                class="drawer-header-btn drawer-header-btn--inactive"
                onclick={() => router.goto('/')}
                aria-label="Home"
              >
                <HomeIcon class="h-[17px] w-[17px] shrink-0" aria-hidden="true" />
              </button>
              <ThemeToggle
                class="drawer-header-btn drawer-header-btn--inactive"
              />
              <label
                for={drawerId}
                class="drawer-header-btn drawer-header-btn--inactive cursor-pointer"
                aria-label="Collapse sidebar"
              >
                <ArrowRightIcon
                  class="h-[17px] w-[17px] shrink-0 transition-transform duration-200 rotate-180"
                  aria-hidden="true"
                />
              </label>
            {/snippet}
          </PaneHeader>

          {#if onRefresh || drawerSearch || drawerChromeToolbar}
            <div class="signal-drawer__search-row">
              {#if onRefresh}
                <div
                  class="shrink-0 {refreshPulse && refreshAsideTip
                    ? 'tooltip tooltip-bottom tooltip-secondary'
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

      <!-- Expanded: list (unmounted when collapsed so footer/count cannot leak) -->
      {#if effectivelyOpen}
      <div
        class="signal-drawer__body"
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
      {/if}

      <!-- Expanded: footer -->
      {#if effectivelyOpen && footer}
        <div class="signal-drawer__footer">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";

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

  .signal-drawer__panel--instant {
    transition: none !important;
  }

  @media (prefers-reduced-motion: reduce) {
    .signal-drawer__panel {
      transition: none !important;
    }
  }

  /* ── Collapsed: open-sidebar toggle pinned to the top ── */
  @keyframes spin-half {
    from { transform: rotate(180deg); }
    to { transform: rotate(0deg); }
  }

  .signal-drawer__collapsed-rail {
    @apply flex shrink-0 flex-col items-center gap-2 px-1.5;
    padding-top: var(--layout-gap);
  }

  .signal-drawer__collapsed-group {
    @apply flex flex-col items-center gap-2;
  }

  /* ── Expanded: unified header panel ── */
  .signal-drawer__header {
    @apply flex w-full min-w-0 shrink-0 flex-col;
  }

  /* Top inset on the header bar (matches page-layout__region). */
  .signal-drawer__header :global(.pane-header.pane-header--flush) {
    @apply relative;
    padding-top: var(--layout-gap);
  }

  /* Chrome vertically centered on the full header strip; tabs stay below. */
  .signal-drawer__header :global(.pane-header__right) {
    @apply absolute inset-y-0 right-0 z-10 flex items-center gap-2 pr-2;
    height: auto;
    margin: 0;
  }

  .signal-drawer__header :global(.pane-header__tab-scroll) {
    padding-right: 7rem;
  }

  /* Refresh + new-data indicator */
  .signal-drawer__refresh {
    @apply relative;
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

  /* ── Search + toolbar row (search · sort · time · refresh) ──
     Top pad = row bottom (pb-2) + signal-row top (py-2) → pt-4 (16px). */
  .signal-drawer__search-row {
    @apply flex min-w-0 w-full shrink-0 items-center gap-2 bg-base-200 px-2 pb-2 pt-4;
  }

  .signal-drawer__search {
    @apply min-w-0 flex-1;
  }

  .signal-drawer__chrome-toolbar {
    @apply flex shrink-0 items-center justify-end gap-2;
  }

  .signal-drawer__search
    :global(.search-editor-wrapper--drawer .search-editor__footer-actions) {
    @apply ml-auto shrink-0 gap-2;
  }

  .signal-drawer__chrome-toolbar :global(.drawer-search-panel) {
    @apply gap-0;
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
    @apply flex shrink-0 items-center bg-base-200 px-3;
    min-height: var(--app-footer-height);
  }

  .signal-drawer__footer > :global(*) {
    @apply min-w-0 flex-1;
  }
</style>
