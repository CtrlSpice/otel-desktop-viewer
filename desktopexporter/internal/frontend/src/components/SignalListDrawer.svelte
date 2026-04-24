<script lang="ts">
  import type { Snippet } from 'svelte'
  import VirtualList from '@humanspeak/svelte-virtual-list'
  import { ArrowLeftIcon, ArrowRightIcon, ArrowDownIcon } from '@/icons'

  type SortOption = { value: string; label: string }

  type Props<T> = {
    items: T[]
    selectedId: string | null
    icon: Snippet
    count: number
    sortOptions: SortOption[]
    sortValue: string
    sortDirection: 'asc' | 'desc'
    storageKey: string
    itemSnippet: Snippet<[item: T, selected: boolean]>
    onSelect?: (id: string) => void
    onSortChange?: (value: string, direction: 'asc' | 'desc') => void
    footer?: Snippet
  }

  let {
    items,
    selectedId,
    icon,
    count,
    sortOptions,
    sortValue,
    sortDirection,
    storageKey,
    itemSnippet,
    onSelect,
    onSortChange,
    footer,
  }: Props<any> = $props()

  const COLLAPSED_KEY_SUFFIX = ':collapsed'
  const WIDTH_KEY_SUFFIX = ':width'
  const DEFAULT_WIDTH = 320
  const MIN_WIDTH = 240
  const MAX_WIDTH = 560
  const RAIL_WIDTH = 48

  let collapsed = $state(loadCollapsed())
  let drawerWidth = $state(loadWidth())
  let isDragging = $state(false)
  let sortDropdownOpen = $state(false)

  function loadCollapsed(): boolean {
    if (typeof localStorage === 'undefined') return false
    return localStorage.getItem(storageKey + COLLAPSED_KEY_SUFFIX) === 'true'
  }

  function loadWidth(): number {
    if (typeof localStorage === 'undefined') return DEFAULT_WIDTH
    const raw = localStorage.getItem(storageKey + WIDTH_KEY_SUFFIX)
    if (!raw) return DEFAULT_WIDTH
    const n = parseInt(raw, 10)
    return n >= MIN_WIDTH && n <= MAX_WIDTH ? n : DEFAULT_WIDTH
  }

  function saveCollapsed() {
    if (typeof localStorage === 'undefined') return
    localStorage.setItem(storageKey + COLLAPSED_KEY_SUFFIX, String(collapsed))
  }

  function saveWidth() {
    if (typeof localStorage === 'undefined') return
    localStorage.setItem(storageKey + WIDTH_KEY_SUFFIX, String(Math.round(drawerWidth)))
  }

  function toggleCollapsed() {
    collapsed = !collapsed
    saveCollapsed()
  }

  let dragStartX = 0
  let dragStartWidth = 0

  function handleResizePointerDown(e: PointerEvent) {
    e.preventDefault()
    const target = e.currentTarget as HTMLElement
    target.setPointerCapture(e.pointerId)
    isDragging = true
    dragStartX = e.clientX
    dragStartWidth = drawerWidth
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
  }

  function handleResizePointerMove(e: PointerEvent) {
    if (!isDragging) return
    const delta = e.clientX - dragStartX
    drawerWidth = Math.max(MIN_WIDTH, Math.min(MAX_WIDTH, dragStartWidth + delta))
  }

  function handleResizePointerUp(e: PointerEvent) {
    if (!isDragging) return
    const target = e.currentTarget as HTMLElement
    target.releasePointerCapture(e.pointerId)
    isDragging = false
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    saveWidth()
  }

  function handleSortSelect(value: string) {
    if (value === sortValue) {
      onSortChange?.(value, sortDirection === 'asc' ? 'desc' : 'asc')
    } else {
      onSortChange?.(value, 'asc')
    }
    sortDropdownOpen = false
  }

  let currentSortLabel = $derived(
    sortOptions.find(o => o.value === sortValue)?.label ?? 'Sort'
  )
</script>

{#if collapsed}
  <!-- Rail mode -->
  <button
    type="button"
    class="drawer-rail"
    onclick={toggleCollapsed}
    aria-label="Expand signal list"
  >
    <span class="drawer-rail__icon">
      {@render icon()}
    </span>
    <span class="drawer-rail__count">{count}</span>
    <ArrowRightIcon class="drawer-rail__chevron" aria-hidden="true" />
  </button>
{:else}
  <!-- Expanded drawer -->
  <div
    class="drawer-expanded"
    style:width="{drawerWidth}px"
    style:min-width="{drawerWidth}px"
  >
    <!-- Header -->
    <div class="drawer-header">
      <div class="drawer-sort">
        <button
          type="button"
          class="drawer-sort__trigger"
          onclick={() => (sortDropdownOpen = !sortDropdownOpen)}
          aria-expanded={sortDropdownOpen}
        >
          <span class="drawer-sort__label">{currentSortLabel}</span>
          <ArrowDownIcon
            class="drawer-sort__chevron {sortDirection === 'asc' ? 'drawer-sort__chevron--asc' : ''}"
            aria-hidden="true"
          />
        </button>
        {#if sortDropdownOpen}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="drawer-sort__dropdown"
            onpointerdown={e => e.stopPropagation()}
          >
            {#each sortOptions as opt (opt.value)}
              <button
                type="button"
                class="drawer-sort__option {opt.value === sortValue ? 'drawer-sort__option--active' : ''}"
                onclick={() => handleSortSelect(opt.value)}
              >
                {opt.label}
                {#if opt.value === sortValue}
                  <ArrowDownIcon
                    class="drawer-sort__option-dir {sortDirection === 'asc' ? 'drawer-sort__option-dir--asc' : ''}"
                    aria-hidden="true"
                  />
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <button
        type="button"
        class="drawer-collapse-btn"
        onclick={toggleCollapsed}
        aria-label="Collapse signal list"
      >
        <ArrowLeftIcon class="h-4 w-4" aria-hidden="true" />
      </button>
    </div>

    <!-- Card list -->
    <div class="drawer-body">
      {#if items.length === 0}
        <div class="drawer-empty">No items</div>
      {:else}
        <VirtualList {items} defaultEstimatedItemHeight={88} bufferSize={10}>
          {#snippet renderItem(item)}
            {@render itemSnippet(item, selectedId === (item as any).id)}
          {/snippet}
        </VirtualList>
      {/if}
    </div>

    <!-- Footer -->
    {#if footer}
      <div class="drawer-footer">
        {@render footer()}
      </div>
    {/if}

    <!-- Resize handle -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div
      class="drawer-resize-handle"
      class:drawer-resize-handle--active={isDragging}
      onpointerdown={handleResizePointerDown}
      onpointermove={handleResizePointerMove}
      onpointerup={handleResizePointerUp}
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize drawer"
      tabindex="0"
    >
      <div class="drawer-resize-handle__line"></div>
    </div>
  </div>
{/if}

{#if sortDropdownOpen}
  <!-- Backdrop to close dropdown -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="drawer-sort-backdrop"
    onclick={() => (sortDropdownOpen = false)}
    onkeydown={e => e.key === 'Escape' && (sortDropdownOpen = false)}
  ></div>
{/if}

<style lang="postcss">
  @reference "../app.css";

  /* ── Rail (collapsed) ── */
  .drawer-rail {
    @apply flex h-full flex-col items-center gap-2 rounded-xl border border-base-300/70 bg-base-100/80 px-2 py-3 shadow-surface-sm backdrop-blur-sm transition-colors;
    width: 48px;
    min-width: 48px;
    cursor: pointer;
  }

  .drawer-rail:hover {
    @apply bg-base-200/60;
  }

  .drawer-rail:focus-visible {
    outline: var(--focus-ring-width) solid var(--focus-ring-color);
    outline-offset: var(--focus-ring-offset);
  }

  .drawer-rail__icon {
    @apply flex h-8 w-8 items-center justify-center text-base-content/70;
  }

  .drawer-rail__icon :global(svg) {
    @apply h-5 w-5;
  }

  .drawer-rail__count {
    @apply text-xs font-semibold tabular-nums text-base-content/60;
  }

  .drawer-rail :global(.drawer-rail__chevron) {
    @apply mt-auto h-4 w-4 text-base-content/40;
  }

  /* ── Expanded drawer ── */
  .drawer-expanded {
    @apply relative flex h-full flex-col overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm;
  }

  .drawer-header {
    @apply flex shrink-0 items-center gap-2 border-b border-base-300/40 px-3 py-2;
  }

  .drawer-sort {
    @apply relative flex-1 min-w-0;
  }

  .drawer-sort__trigger {
    @apply flex w-full items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium text-base-content/70 transition-colors hover:bg-base-200/60 hover:text-base-content;
  }

  .drawer-sort__label {
    @apply min-w-0 truncate;
  }

  .drawer-sort__trigger :global(.drawer-sort__chevron) {
    @apply h-3 w-3 shrink-0 transition-transform duration-200;
  }

  .drawer-sort__trigger :global(.drawer-sort__chevron--asc) {
    @apply rotate-180;
  }

  .drawer-sort__dropdown {
    @apply absolute left-0 top-full z-50 mt-1 w-full min-w-[8rem] rounded-md border border-base-300 bg-base-100 py-1 shadow-lg;
  }

  .drawer-sort__option {
    @apply flex w-full items-center justify-between px-3 py-1.5 text-xs text-base-content/80 transition-colors hover:bg-base-200/60;
  }

  .drawer-sort__option--active {
    @apply text-primary font-medium;
  }

  .drawer-sort__option :global(.drawer-sort__option-dir) {
    @apply h-3 w-3 shrink-0 text-primary transition-transform duration-200;
  }

  .drawer-sort__option :global(.drawer-sort__option-dir--asc) {
    @apply rotate-180;
  }

  .drawer-collapse-btn {
    @apply flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-base-content/50 transition-colors hover:bg-base-200/60 hover:text-base-content;
  }

  .drawer-collapse-btn:focus-visible {
    outline: var(--focus-ring-width) solid var(--focus-ring-color);
    outline-offset: var(--focus-ring-offset);
  }

  /* ── Body (virtual scroll) ── */
  .drawer-body {
    @apply flex-1 min-h-0 overflow-hidden;
  }

  .drawer-body :global(.virtual-list-wrapper) {
    @apply h-full;
  }

  .drawer-body :global(.virtual-list-viewport) {
    @apply h-full px-2 py-1;
    scrollbar-width: thin;
  }

  .drawer-body :global(.virtual-list-items > *) {
    @apply py-0.5;
  }

  .drawer-empty {
    @apply flex h-full items-center justify-center text-sm text-base-content/40;
  }

  /* ── Footer ── */
  .drawer-footer {
    @apply shrink-0 border-t border-base-300/40 px-3 py-2;
  }

  /* ── Resize handle ── */
  .drawer-resize-handle {
    @apply absolute right-0 top-0 bottom-0 z-10 flex cursor-col-resize items-stretch justify-center;
    width: 6px;
    margin-right: -3px;
  }

  .drawer-resize-handle__line {
    @apply self-stretch rounded-full;
    width: 2px;
    background-color: var(--color-neutral);
    opacity: 0;
    transition: opacity 0.15s, background-color 0.15s;
  }

  .drawer-resize-handle:hover .drawer-resize-handle__line {
    opacity: 0.3;
  }

  .drawer-resize-handle--active .drawer-resize-handle__line {
    background-color: var(--color-primary);
    opacity: 0.5;
  }

  .drawer-resize-handle:focus-visible {
    outline: none;
  }

  .drawer-resize-handle:focus-visible .drawer-resize-handle__line {
    background-color: var(--color-primary);
    opacity: 0.9;
  }

  /* ── Sort backdrop ── */
  .drawer-sort-backdrop {
    @apply fixed inset-0 z-40;
  }
</style>
