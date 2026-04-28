<script lang="ts">
  import type { Snippet } from 'svelte'
  import VirtualList from '@humanspeak/svelte-virtual-list'
  import { ArrowDownIcon, ArrowUpIcon } from '@/icons'

  type SortOption = { value: string; label: string }

  type Props<T> = {
    items: T[]
    selectedId: string | null
    drawerId: string
    icon: Snippet
    label: string
    count: number
    sortOptions: SortOption[]
    sortValue: string
    sortDirection: 'asc' | 'desc'
    storageKey: string
    itemSnippet: Snippet<[item: T, selected: boolean]>
    itemKey?: (item: T) => string
    onSelect?: (id: string) => void
    onSortChange?: (value: string, direction: 'asc' | 'desc') => void
    footer?: Snippet
    children: Snippet
  }

  let {
    items,
    selectedId,
    drawerId,
    icon,
    label,
    count,
    sortOptions,
    sortValue,
    sortDirection,
    storageKey,
    itemSnippet,
    itemKey = (item: any) => item.id,
    onSelect,
    onSortChange,
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

  let currentSortLabel = $derived(
    sortOptions.find(o => o.value === sortValue)?.label ?? 'Sort'
  )

  function selectSort(value: string, dir: 'asc' | 'desc') {
    onSortChange?.(value, dir)
    // Close the details dropdown after selection
    const el = document.querySelector('.signal-drawer__sort-dropdown') as HTMLDetailsElement | null
    if (el) el.open = false
  }
</script>

<div class="signal-drawer drawer drawer-open">
  <input
    id={drawerId}
    type="checkbox"
    class="drawer-toggle"
    checked={drawerOpen}
    onchange={handleToggleChange}
  />

  <div class="drawer-content min-h-0 min-w-0">
    {@render children()}
  </div>

  <div class="drawer-side is-drawer-close:overflow-visible">
    <div
      class="signal-drawer__panel flex h-full flex-col bg-base-100/95 is-drawer-close:w-14 is-drawer-open:w-72"
    >
      <!-- Header: toggle (always visible) + title + sort (open only) -->
      <div class="signal-drawer__header">
        <label
          for={drawerId}
          class="signal-drawer__toggle is-drawer-close:tooltip is-drawer-close:tooltip-right"
          data-tip={label}
          aria-label={drawerOpen ? 'Close sidebar' : 'Open sidebar'}
        >
          <span class="signal-drawer__toggle-icon">
            {@render icon()}
          </span>
        </label>

        <details class="dropdown signal-drawer__sort-dropdown is-drawer-close:hidden">
          <summary class="signal-drawer__sort-trigger">
            <span class="signal-drawer__sort-prefix">Order by:</span>
            <span class="signal-drawer__sort-value">{currentSortLabel}</span>
            {#if sortDirection === 'asc'}
              <ArrowUpIcon class="signal-drawer__sort-dir-indicator" aria-hidden="true" />
            {:else}
              <ArrowDownIcon class="signal-drawer__sort-dir-indicator" aria-hidden="true" />
            {/if}
          </summary>
          <ul class="menu dropdown-content bg-base-100 rounded-box z-50 w-48 p-1 shadow-lg border border-base-300">
            {#each sortOptions as opt (opt.value)}
              <li>
                <button
                  type="button"
                  class="signal-drawer__sort-option {opt.value === sortValue ? 'signal-drawer__sort-option--active' : ''}"
                  onclick={() => selectSort(opt.value, opt.value === sortValue && sortDirection === 'asc' ? 'desc' : 'asc')}
                >
                  <span>{opt.label}</span>
                  {#if opt.value === sortValue}
                    {#if sortDirection === 'asc'}
                      <ArrowUpIcon class="signal-drawer__sort-dir" aria-hidden="true" />
                    {:else}
                      <ArrowDownIcon class="signal-drawer__sort-dir" aria-hidden="true" />
                    {/if}
                  {/if}
                </button>
              </li>
            {/each}
          </ul>
        </details>
      </div>

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
    @apply h-full;
    min-height: 0;
  }

  .signal-drawer__panel {
    @apply transition-[width] duration-200;
    border-right: 1px solid color-mix(in oklab, var(--color-base-300) 70%, transparent);
  }

  /* ── Header ── */
  .signal-drawer__header {
    @apply flex shrink-0 items-center gap-1 border-b border-base-300/40 px-1.5 py-2;
  }

  .signal-drawer__toggle {
    @apply flex h-8 w-8 shrink-0 cursor-pointer items-center justify-center rounded-md text-base-content/70 transition-colors hover:bg-base-200/60 hover:text-base-content;
  }

  .signal-drawer__toggle-icon {
    @apply flex items-center justify-center;
  }

  .signal-drawer__toggle-icon :global(svg) {
    @apply h-4 w-4;
  }

  /* ── Sort dropdown ── */
  .signal-drawer__sort-dropdown {
    @apply flex-1 min-w-0;
  }

  .signal-drawer__sort-trigger {
    @apply flex w-full cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-base-content/70 transition-colors;
    @apply hover:bg-base-200/60 hover:text-base-content;
    list-style: none;
  }

  .signal-drawer__sort-trigger::-webkit-details-marker {
    display: none;
  }

  .signal-drawer__sort-prefix {
    @apply shrink-0 text-base-content/40;
  }

  .signal-drawer__sort-value {
    @apply min-w-0 truncate font-medium;
  }

  .signal-drawer__sort-trigger :global(.signal-drawer__sort-dir-indicator) {
    @apply ml-auto h-3.5 w-3.5 shrink-0 text-base-content/50;
  }

  .signal-drawer__sort-option {
    @apply flex w-full items-center justify-between gap-2 text-xs;
  }

  .signal-drawer__sort-option--active {
    @apply text-primary font-medium;
  }

  .signal-drawer__sort-option :global(.signal-drawer__sort-dir) {
    @apply h-3.5 w-3.5 shrink-0 text-primary;
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
