<script lang="ts">
  import SignalFooter from './SignalFooter.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    ChevronLeftIcon,
    ChevronRightIcon,
    Delete02Icon,
    ArrowLeftDoubleIcon,
    ArrowRightDoubleIcon,
  } from '@hugeicons/core-free-icons'

  let {
    page,
    pageSize,
    total,
    loading = false,
    onPage,
    onPageSize,
    onDeleteAll,
    selectionMode = false,
    selectedCount = 0,
    deleting = false,
    onDeleteSelected,
  }: {
    page: number
    pageSize: number
    total: number
    loading?: boolean
    onPage: (page: number) => void
    onPageSize: (size: number) => void
    onDeleteAll?: () => void
    selectionMode?: boolean
    selectedCount?: number
    deleting?: boolean
    onDeleteSelected?: () => void
  } = $props()

  let pages = $derived(Math.max(1, Math.ceil(total / pageSize)))
  let visible = $derived(
    [
      ...new Set([
        1,
        ...Array.from({ length: 5 }, (_, i) => page - 2 + i),
        pages,
      ]),
    ]
      .filter(value => value >= 1 && value <= pages)
      .sort((a, b) => a - b)
  )
</script>

<div class="pagination">
  <SignalFooter
    index={page - 1}
    total={pages}
    label="log"
    onFirst={() => onPage(1)}
    onPrev={() => onPage(page - 1)}
    onNext={() => onPage(page + 1)}
    onLast={() => onPage(pages)}
  >
    {#snippet leading()}
      <div class="pagination__summary">
        {#if onDeleteAll && !selectionMode}<button
            type="button"
            class="btn btn-ghost btn-sm btn-square text-error tooltip tooltip-right"
            aria-label="Delete all logs"
            data-tip="Delete all logs"
            onclick={onDeleteAll}
            ><HugeiconsIcon
              icon={Delete02Icon}
              size="1em"
              strokeWidth={1.5}
              class="h-3.5 w-3.5"
              aria-hidden="true"
            /></button
          >{/if}
        <span role="status" aria-label="Log count" class="tabular-nums"
          >{total.toLocaleString()} {total === 1 ? 'log' : 'logs'}</span
        >
      </div>
    {/snippet}
    {#snippet navigation()}
      <nav aria-label="Log pages" class="pagination__pages">
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-square tooltip tooltip-top"
          data-tip="First page"
          aria-label="First page"
          disabled={loading || page <= 1}
          onclick={() => onPage(1)}
        >
          <HugeiconsIcon
            icon={ArrowLeftDoubleIcon}
            size="1em"
            strokeWidth={1.5}
            class="h-3.5 w-3.5"
            aria-hidden="true"
          />
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-square tooltip tooltip-top"
          data-tip="Previous page"
          aria-label="Previous page"
          disabled={loading || page <= 1}
          onclick={() => onPage(page - 1)}
          ><HugeiconsIcon
            icon={ChevronLeftIcon}
            size="1em"
            strokeWidth={1.5}
            class="h-3.5 w-3.5"
            aria-hidden="true"
          /></button
        >
        {#each visible as number, index (number)}
          {#if index > 0 && number > visible[index - 1]! + 1}<span
              aria-hidden="true">…</span
            >{/if}
          <button
            type="button"
            class="btn btn-ghost btn-sm btn-square text-sm font-normal tabular-nums"
            class:btn-active={number === page}
            aria-label={`Page ${number}`}
            aria-current={number === page ? 'page' : undefined}
            disabled={loading}
            onclick={() => onPage(number)}>{number}</button
          >
        {/each}
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-square tooltip tooltip-top"
          data-tip="Next page"
          aria-label="Next page"
          disabled={loading || page >= pages}
          onclick={() => onPage(page + 1)}
          ><HugeiconsIcon
            icon={ChevronRightIcon}
            size="1em"
            strokeWidth={1.5}
            class="h-3.5 w-3.5"
            aria-hidden="true"
          /></button
        >
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-square tooltip tooltip-top"
          data-tip="Last page"
          aria-label="Last page"
          disabled={loading || page >= pages}
          onclick={() => onPage(pages)}
        >
          <HugeiconsIcon
            icon={ArrowRightDoubleIcon}
            size="1em"
            strokeWidth={1.5}
            class="h-3.5 w-3.5"
            aria-hidden="true"
          />
        </button>
      </nav>
    {/snippet}
    {#snippet trailing()}
      {#if selectionMode}
        <button
          type="button"
          class="btn btn-ghost btn-sm text-sm font-normal"
          disabled={!selectedCount || deleting || loading}
          onclick={onDeleteSelected}
        >
          Delete {selectedCount} selected {selectedCount === 1 ? 'log' : 'logs'}
        </button>
      {/if}
      <div class="pagination__size">
        <label class="flex items-center gap-2 text-xs text-subtle"
          >Rows
          <select
            class="select select-ghost select-xs w-16 text-xs"
            aria-label="Logs per page"
            value={pageSize}
            disabled={loading}
            onchange={event => onPageSize(Number(event.currentTarget.value))}
          >
            {#each [25, 50, 100, 250] as size}<option value={size}
                >{size}</option
              >{/each}
          </select>
        </label>
      </div>
    {/snippet}
  </SignalFooter>
</div>

<style lang="postcss">
  @reference "../../app.css";
  .pagination {
    flex-shrink: 0;
    container-type: inline-size;
  }
  @container (width < 60rem) {
    .pagination :global(.signal-footer) {
      grid-template-columns: minmax(0, 1fr) auto;
    }
    .pagination :global(.signal-footer > :nth-child(2)) {
      grid-column: 1 / -1;
      grid-row: 2;
    }
    .pagination :global(.signal-footer > :last-child) {
      grid-column: 2;
      grid-row: 1;
    }
  }
  .pagination__summary,
  .pagination__pages,
  .pagination__size {
    @apply flex min-w-0 items-center gap-1;
  }
  .pagination__pages {
    @apply flex-wrap justify-center;
  }
  .pagination__summary {
    @apply text-xs text-base-content;
  }
</style>
