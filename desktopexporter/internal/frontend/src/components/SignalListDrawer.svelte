<script lang="ts">
  import type { Snippet } from 'svelte'
  import { setContext } from 'svelte'
  import VirtualList from '@humanspeak/svelte-virtual-list'
  import { ArrowRightIcon } from '@/icons'
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
    refreshAside?: Snippet
    drawerChrome?: Snippet
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
    refreshAside,
    drawerChrome,
    drawerSearch,
    footer,
    children,
  }: Props<any> = $props()

  let drawerOpen = $state(loadOpen())

  function loadOpen(): boolean {
    if (typeof localStorage === 'undefined') return true
    const v = localStorage.getItem(storageKey + ':open')
    return v === null ? true : v === 'true'
  }

  function handleToggleChange(e: Event) {
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
    drawerChromeContext.closeForId = drawerOpen ? drawerId : undefined
  })
</script>

<div class="signal-drawer drawer drawer-open">
  <input
    id={drawerId}
    type="checkbox"
    class="drawer-toggle signal-drawer-toggle"
    checked={drawerOpen}
    onchange={handleToggleChange}
  />

  <div class="drawer-content min-h-0 min-w-0">
    {@render children()}
  </div>

  <div class="drawer-side is-drawer-close:overflow-visible">
    <div
      class="signal-drawer__panel flex h-full flex-col bg-base-100/95 is-drawer-close:w-14 is-drawer-open:w-96"
    >
      {#if !drawerOpen}
        <!-- Collapsed: open-sidebar toggle pinned to the very top -->
        <div class="signal-drawer__open-toggle">
          <label
            for={drawerId}
            class="nav-button nav-button-icon-only nav-button-inactive tooltip tooltip-right cursor-pointer"
            data-tip={label}
            aria-label="Open sidebar"
          >
            <ArrowRightIcon class="h-[17px] w-[17px]" aria-hidden="true" />
          </label>
        </div>
      {/if}

      {#if !drawerOpen}
        <!-- Collapsed: primary nav (icons) -->
        <div class="signal-drawer__nav signal-drawer__nav--collapsed">
          <DrawerNavTabs collapsed />
        </div>
      {/if}

      <!-- Collapsed: header controls (refresh, aside, theme toggle) -->
      {#if !drawerOpen}
        <div class="signal-drawer__header signal-drawer__header--collapsed">
          <div
            class="signal-drawer__header-controls--collapsed flex shrink-0 flex-col items-center gap-1.5"
          >
            {#if onRefresh}
              <button
                type="button"
                class="signal-drawer__refresh nav-button nav-button-icon-only nav-button-inactive shrink-0"
                class:signal-drawer__refresh--has-new-data={refreshPulse}
                onclick={onRefresh}
                aria-label={refreshPulse
                  ? 'Refresh — incoming data pending'
                  : 'Refresh'}
                title={refreshPulse
                  ? 'New data pending — reload to merge'
                  : 'Refresh'}
              >
                <svg
                  class="h-[17px] w-[17px] shrink-0"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="1.5"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <path
                    d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                  />
                </svg>
              </button>
            {/if}
            {#if onRefresh && refreshAside && refreshPulse}
              <div
                class="signal-drawer__refresh-aside-row signal-drawer__refresh-aside-row--collapsed"
                aria-live="polite"
              >
                {@render refreshAside()}
              </div>
            {/if}
            <ThemeToggle />
          </div>
        </div>
      {/if}

      <!-- Expanded: tinted toolbar strip; lift tabs sit on panel surface below (lift needs base-100) -->
      {#if drawerOpen}
        <div class="signal-drawer__chrome-stack is-drawer-close:hidden">
          <DrawerNavTabs />
          {#if drawerChrome}
            <div class="signal-drawer__toolbar-slot">
              {@render drawerChrome()}
            </div>
          {/if}
        </div>
      {/if}

      {#if drawerSearch}
        <div class="signal-drawer__search is-drawer-close:hidden">
          {@render drawerSearch()}
        </div>
      {/if}

      <!-- Collapsed: count badge -->
      <div class="signal-drawer__rail-count is-drawer-open:hidden">
        {count}
      </div>

      <!-- Expanded: list -->
      <div class="signal-drawer__body is-drawer-close:hidden">
        {#if items.length === 0}
          <div class="signal-drawer__empty">No items</div>
        {:else}
          <VirtualList
            {items}
            defaultEstimatedItemHeight={52}
            bufferSize={10}
            containerClass="signal-drawer__vlist"
            viewportClass="signal-drawer__vlist-viewport"
            itemsClass="signal-drawer__vlist-items"
          >
            {#snippet renderItem(item)}
              {@render itemSnippet(item, selectedId === itemKey(item))}
            {/snippet}
          </VirtualList>
        {/if}
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

  /* ── Expanded: toolbar-only tint (tabs use lift on panel bg below) ── */
  .signal-drawer__chrome-stack {
    @apply flex shrink-0 items-center gap-1 bg-base-200/55 px-2 py-1;
  }

  .signal-drawer__chrome-stack :global(.drawer-nav-tabs) {
    @apply flex-1 min-w-0;
  }

  .signal-drawer__toolbar-slot {
    @apply shrink-0;
  }

  /* ── Header (collapsed only) ── */
  .signal-drawer__header {
    @apply flex w-full min-w-0 shrink-0 px-2 pt-2 pb-1;
  }

  .signal-drawer__header--collapsed {
    @apply flex flex-col items-center justify-center gap-1.5 py-2;
  }

  /* Refresh + new-data indicator*/
  .signal-drawer__refresh {
    @apply relative rounded-lg;
  }

  @keyframes signal-drawer-refresh-new-glow {
    0%,
    100% {
      box-shadow:
        0 0 0 1px color-mix(in oklab, var(--color-primary) 18%, transparent),
        0 0 10px color-mix(in oklab, var(--color-primary) 12%, transparent);
    }
    50% {
      box-shadow:
        0 0 0 1px color-mix(in oklab, var(--color-primary) 38%, transparent),
        0 0 22px color-mix(in oklab, var(--color-primary) 28%, transparent);
    }
  }

  .signal-drawer__refresh.signal-drawer__refresh--has-new-data:not(:hover):not(
      :focus-visible
    ) {
    animation: signal-drawer-refresh-new-glow 2.8s ease-in-out infinite;
  }

  @media (prefers-reduced-motion: reduce) {
    .signal-drawer__refresh.signal-drawer__refresh--has-new-data {
      animation: none !important;
    }
  }

  /** Full-width row below header controls — counts inline, comma-separated */
  .signal-drawer__refresh-aside-row {
    @apply flex min-w-0 flex-wrap items-baseline justify-start gap-y-1 text-xs text-primary/75;
  }

  .signal-drawer__refresh-aside-row
    :global(.signal-drawer__refresh-aside-pill) {
    @apply inline-flex max-w-full items-center whitespace-nowrap tabular-nums leading-snug;
  }

  .signal-drawer__refresh-aside-row
    :global(.signal-drawer__refresh-aside-pill:not(:first-child)::before) {
    content: ', ';
  }

  .signal-drawer__refresh-aside-row--collapsed {
    @apply w-full justify-start px-px;
    flex-wrap: wrap;
  }

  .signal-drawer__refresh-aside-row--collapsed
    :global(.signal-drawer__refresh-aside-pill) {
    @apply max-w-full text-left text-xs leading-tight;
    white-space: normal;
    word-break: break-word;
  }

  /* ── Search block (under tab-content — no top border/rounding needed) ── */
  .signal-drawer__search {
    @apply shrink-0 border-b border-base-300/40 bg-base-200/55 px-2 pt-0 pb-2;
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

  .signal-drawer__empty {
    @apply flex h-full items-center justify-center text-sm text-base-content/40;
  }

  /* ── Footer ── */
  .signal-drawer__footer {
    @apply shrink-0 border-t border-base-300/40 px-3 py-2;
  }
</style>
