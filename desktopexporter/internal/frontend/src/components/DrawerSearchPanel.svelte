<script lang="ts">
  import { ArrowDownIcon, ArrowUpIcon } from '@/icons'
  import HugeiconsSorting05 from '@/icons/HugeiconsSorting05.svelte'
  import DateTimeFilter from '@/components/SignalToolbar/datetime/DateTimeFilter.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import type { SearchResultEvent } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/SignalToolbar/search/search-editor-api'

  type SortOption = { value: string; label: string }

  type DrawerSearchPanelSegment = 'full' | 'toolbar' | 'search'

  type Props = {
    /** `toolbar` = sort/time/refresh · `search` = editor · `full` = both */
    segment?: DrawerSearchPanelSegment
    signal: 'traces' | 'metrics' | 'logs'
    sortOptions: SortOption[]
    sortValue: string
    sortDirection: 'asc' | 'desc'
    onSortChange?: (value: string, direction: 'asc' | 'desc') => void
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
    onSearchResults,
    onSearchError,
    onSearchReady,
  }: Props = $props()

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
  {#if segment === 'full' || segment === 'toolbar'}
    <!-- Toolbar row: time · sort -->
    <div
      class="drawer-search-panel__toolbar-row"
      role="toolbar"
      aria-label="List controls"
    >
      <DateTimeFilter
        triggerVariant="icon"
        class="drawer-header-btn drawer-header-btn--inactive shrink-0"
      />

      <details
        class="dropdown drawer-search-panel__sort-dropdown shrink-0"
      >
        <summary
          class="drawer-header-btn drawer-header-btn--inactive drawer-search-panel__sort-summary"
          title={`Sort: ${currentSortLabel} (${sortDirection})`}
        >
          <HugeiconsSorting05 class="h-[17px] w-[17px] shrink-0" />
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
                class="drawer-search-panel__sort-option {opt.value ===
                sortValue
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
    </div>
  {/if}

  {#if segment === 'full' || segment === 'search'}
    <SearchEditor
      {signal}
      variant="drawer"
      {onSearchResults}
      {onSearchError}
      onReady={onSearchReady}
    />
  {/if}
</div>

<style lang="postcss">
  @reference "../app.css";

  .drawer-search-panel {
    @apply flex w-full min-w-0 flex-col gap-1;
  }

  .drawer-search-panel__toolbar-row {
    @apply flex min-w-0 items-center gap-1;
  }

  /* Sort dropdown */
  .drawer-search-panel__sort-summary {
    list-style: none;
  }

  .drawer-search-panel__sort-summary::-webkit-details-marker {
    display: none;
  }

  .drawer-search-panel__sort-dropdown[open]
    > .drawer-search-panel__sort-summary {
    @apply border-transparent bg-primary/15 text-primary shadow-sm shadow-primary/10;
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
