<script lang="ts">
  import { ArrowUpIcon } from '@/icons'
  import HugeiconsSorting05 from '@/icons/HugeiconsSorting05.svelte'
  import DateTimeFilter from '@/components/SignalToolbar/datetime/DateTimeFilter.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import {
    createPopoverId,
    setupAnchorPopover,
  } from '@/utils/anchor-popover'
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

  let sortPopoverEl = $state<HTMLDivElement | null>(null)
  let sortTriggerEl = $state<HTMLButtonElement | null>(null)
  let sortPopoverOpen = $state(false)

  const sortPopoverId = createPopoverId('sort-popover')

  let currentSortLabel = $derived(
    sortOptions.find(o => o.value === sortValue)?.label ?? 'Sort'
  )

  let sortAriaLabel = $derived(
    `Sort by ${currentSortLabel}, ${sortDirection === 'asc' ? 'ascending' : 'descending'}`
  )

  $effect(() => {
    const popover = sortPopoverEl
    const trigger = sortTriggerEl
    if (!popover || !trigger) return
    return setupAnchorPopover({
      popover,
      trigger,
      anchor: 'below-end',
      onOpenChange: open => {
        sortPopoverOpen = open
      },
    })
  })

  function selectSort(value: string, dir: 'asc' | 'desc') {
    onSortChange?.(value, dir)
    sortPopoverEl?.hidePopover()
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
        class="drawer-header-btn drawer-header-btn--inactive shrink-0"
      />

      <button
        bind:this={sortTriggerEl}
        type="button"
        class="drawer-header-btn drawer-header-btn--inactive shrink-0"
        popovertarget={sortPopoverId}
        aria-expanded={sortPopoverOpen}
        aria-label={sortAriaLabel}
      >
        <HugeiconsSorting05 class="h-[17px] w-[17px] shrink-0" />
      </button>

      <div
        bind:this={sortPopoverEl}
        popover="auto"
        id={sortPopoverId}
        class="anchor-popover anchor-popover--anchored anchor-popover--menu"
      >
        <ul class="anchor-popover-menu" role="menu" aria-label="Sort by">
          {#each sortOptions as opt (opt.value)}
            <li role="none">
              <button
                type="button"
                role="menuitemradio"
                aria-checked={opt.value === sortValue}
                class="anchor-popover-menu__option {opt.value === sortValue
                  ? 'anchor-popover-menu__option--active'
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
                  <ArrowUpIcon
                    class="anchor-popover-menu__option-icon {sortDirection === 'desc'
                      ? 'rotate-180'
                      : ''}"
                    aria-hidden="true"
                  />
                {/if}
              </button>
            </li>
          {/each}
        </ul>
      </div>
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
    @apply flex w-full min-w-0 flex-col gap-2;
  }

  .drawer-search-panel__toolbar-row {
    @apply flex min-w-0 items-center justify-end gap-2;
  }
</style>
