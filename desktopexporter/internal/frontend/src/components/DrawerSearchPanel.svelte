<script lang="ts">
  import type { Snippet } from 'svelte'
  import {
    ArrowDownIcon,
    ArrowLeftIcon,
    ArrowUpIcon,
  } from '@/icons'
  import HugeiconsSorting05 from '@/icons/HugeiconsSorting05.svelte'
  import DateTimeFilter from '@/components/SignalToolbar/datetime/DateTimeFilter.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import ThemeToggle from '@/components/ThemeToggle.svelte'
  import type { SearchResultEvent } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/SignalToolbar/search/search-editor-api'
  import { getSignalDrawerChrome } from '@/contexts/signal-drawer-chrome.svelte'
  import { HOME_NAV, isNavItemActive } from '@/components/DrawerNavTabs.svelte'
  import { router } from 'tinro5'

  type SortOption = { value: string; label: string }

  type DrawerSearchPanelSegment = 'full' | 'chrome' | 'search'

  type Props = {
    /** `full` = chrome + search (default). Drawer splits into `chrome` then `search` under tabs. */
    segment?: DrawerSearchPanelSegment
    signal: 'traces' | 'metrics' | 'logs'
    sortOptions: SortOption[]
    sortValue: string
    sortDirection: 'asc' | 'desc'
    onSortChange?: (value: string, direction: 'asc' | 'desc') => void
    onRefresh?: () => void
    refreshPulse?: boolean
    refreshAside?: Snippet
    onSearchResults?: (event: SearchResultEvent) => void
    onSearchError?: (error: string | null) => void
    onSearchReady?: (api: SearchEditorAPI) => void
  }

  let {
    segment = 'full',
    signal,
    sortOptions,
    sortValue,
    sortDirection,
    onSortChange,
    onRefresh,
    refreshPulse = false,
    refreshAside,
    onSearchResults,
    onSearchError,
    onSearchReady,
  }: Props = $props()

  const drawerChrome = getSignalDrawerChrome()

  let currentPath = $state(router.path ?? '/')
  $effect(() => {
    const unsubscribe = router.subscribe(route => {
      currentPath = route.path
    })
    return unsubscribe
  })
  let homeActive = $derived(isNavItemActive(HOME_NAV.id, currentPath))
  const HomeIcon = HOME_NAV.icon

  let currentSortLabel = $derived(
    sortOptions.find(o => o.value === sortValue)?.label ?? 'Sort'
  )

  function selectSort(value: string, dir: 'asc' | 'desc') {
    onSortChange?.(value, dir)
    const el = document.querySelector(
      '.drawer-search-panel__sort-dropdown'
    ) as HTMLDetailsElement | null
    if (el) el.open = false
  }
</script>

<div class="drawer-search-panel">
  {#if segment === 'full' || segment === 'chrome'}
  <!-- Chrome row: home · theme · collapse -->
  <div
    class="drawer-search-panel__chrome-row"
    role="group"
    aria-label="Drawer chrome"
  >
    {#if drawerChrome}
      <button
        type="button"
        class="nav-button nav-button-icon-only shrink-0 {homeActive ? 'nav-button-active' : 'nav-button-inactive'}"
        aria-label="Home"
        title="Home"
        aria-current={homeActive ? 'page' : undefined}
        onclick={() => router.goto(HOME_NAV.path)}
      >
        <HomeIcon class="h-[17px] w-[17px] shrink-0" aria-hidden="true" />
      </button>

      <!-- Must not use .nav-button on ThemeToggle: flex + gap breaks Daisy swap -->
      <ThemeToggle
        class="btn btn-ghost btn-neutral btn-sm btn-square shrink-0 border-transparent shadow-none transition-[color,background-color] duration-200"
      />

      {#if drawerChrome.closeForId}
        <label
          for={drawerChrome.closeForId}
          class="nav-button nav-button-icon-only nav-button-inactive shrink-0 cursor-pointer"
          aria-label="Close sidebar"
          title="Collapse sidebar"
        >
          <ArrowLeftIcon class="h-[17px] w-[17px] shrink-0" aria-hidden="true" />
        </label>
      {/if}
    {/if}
  </div>
  {/if}

  {#if segment === 'full' || segment === 'search'}
  <!-- Search editor with signal buttons pinned top-right -->
  <SearchEditor
    {signal}
    variant="drawer"
    onSearchResults={onSearchResults}
    onSearchError={onSearchError}
    onReady={onSearchReady}
  >
    {#snippet headerActions()}
      <div class="flex w-full items-center">
        <details class="dropdown drawer-search-panel__sort-dropdown shrink-0">
          <summary
            class="btn btn-ghost btn-xs btn-square drawer-search-panel__sort-summary shrink-0 text-base-content/55 hover:bg-base-200/80 hover:text-base-content"
            title={`Sort: ${currentSortLabel} (${sortDirection})`}
          >
            <HugeiconsSorting05 class="h-3.5 w-3.5 shrink-0" />
            <span class="sr-only">
              Sort by {currentSortLabel},
              {sortDirection === 'asc' ? 'ascending' : 'descending'}
            </span>
          </summary>
          <ul
            class="menu dropdown-content z-50 w-48 rounded-box border border-base-300 bg-base-100 p-1 shadow-lg"
          >
            {#each sortOptions as opt (opt.value)}
              <li>
                <button
                  type="button"
                  class="drawer-search-panel__sort-option {opt.value === sortValue
                    ? 'drawer-search-panel__sort-option--active'
                    : ''}"
                  onclick={() =>
                    selectSort(
                      opt.value,
                      opt.value === sortValue && sortDirection === 'asc'
                        ? 'desc'
                        : 'asc'
                    )}
                >
                  <span>{opt.label}</span>
                  {#if opt.value === sortValue}
                    {#if sortDirection === 'asc'}
                      <ArrowUpIcon
                        class="drawer-search-panel__sort-dir"
                        aria-hidden="true"
                      />
                    {:else}
                      <ArrowDownIcon
                        class="drawer-search-panel__sort-dir"
                        aria-hidden="true"
                      />
                    {/if}
                  {/if}
                </button>
              </li>
            {/each}
          </ul>
        </details>

        <DateTimeFilter
          triggerVariant="icon"
          class="btn btn-ghost btn-xs btn-square shrink-0 text-base-content/55 hover:bg-base-200/80 hover:text-base-content"
        />

        <div class="flex-1"></div>

        {#if onRefresh}
          <button
            type="button"
            class="btn btn-ghost btn-xs btn-square drawer-search-panel__refresh shrink-0 text-base-content/55 hover:bg-base-200/80 hover:text-base-content"
            class:drawer-search-panel__refresh--pulse={refreshPulse}
            onclick={onRefresh}
            aria-label={refreshPulse ? 'Refresh — incoming data pending' : 'Refresh'}
            title={refreshPulse ? 'New data pending — reload to merge' : 'Refresh'}
          >
            {#if refreshPulse}
              <span
                class="drawer-search-panel__new-data-dot"
                aria-hidden="true"
              ></span>
            {/if}
            <svg
              class="relative z-[1] h-3.5 w-3.5 shrink-0"
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
      </div>

      {#if onRefresh && refreshAside && refreshPulse}
        <div class="drawer-search-panel__refresh-aside" aria-live="polite">
          {@render refreshAside()}
        </div>
      {/if}
    {/snippet}
  </SearchEditor>
  {/if}
</div>

<style lang="postcss">
  @reference "../app.css";

  .drawer-search-panel {
    @apply flex w-full min-w-0 flex-col gap-1.5;
  }

  .drawer-search-panel__chrome-row {
    @apply flex items-center justify-end gap-1;
  }

  /* Refresh button + pulse animation */
  .drawer-search-panel__refresh {
    @apply relative bg-transparent;
  }

  .drawer-search-panel__new-data-dot {
    @apply pointer-events-none absolute right-0.5 top-0.5 z-[2] size-2 rounded-full bg-primary shadow-sm ring-2 ring-base-100;
  }

  @keyframes drawer-refresh-pulse {
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

  .drawer-search-panel__refresh.drawer-search-panel__refresh--pulse:not(:hover):not(
      :focus-visible
    ) {
    animation: drawer-refresh-pulse 2.8s ease-in-out infinite;
  }

  @media (prefers-reduced-motion: reduce) {
    .drawer-search-panel__refresh.drawer-search-panel__refresh--pulse {
      animation: none !important;
    }
  }

  /* Refresh aside row (pending counts) */
  .drawer-search-panel__refresh-aside {
    @apply flex min-w-0 flex-wrap items-baseline justify-start gap-y-1 text-xs text-primary/75;
  }

  .drawer-search-panel__refresh-aside
    :global(.signal-drawer__refresh-aside-pill) {
    @apply inline-flex max-w-full items-center whitespace-nowrap tabular-nums leading-snug;
  }

  .drawer-search-panel__refresh-aside
    :global(.signal-drawer__refresh-aside-pill:not(:first-child)::before) {
    content: ', ';
  }

  /* Sort dropdown */
  .drawer-search-panel__sort-summary {
    list-style: none;
  }

  .drawer-search-panel__sort-summary::-webkit-details-marker {
    display: none;
  }

  .drawer-search-panel__sort-dropdown[open] > .drawer-search-panel__sort-summary {
    @apply border-transparent bg-primary/15 text-primary shadow-sm shadow-primary/10 ring-1 ring-primary/20;
  }

  .drawer-search-panel__sort-option {
    @apply flex w-full items-center justify-between gap-2 text-xs;
  }

  .drawer-search-panel__sort-option--active {
    @apply text-primary font-medium;
  }

  .drawer-search-panel__sort-option :global(.drawer-search-panel__sort-dir) {
    @apply h-3.5 w-3.5 shrink-0 text-primary;
  }
</style>
