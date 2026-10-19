<script lang="ts">
  import SignalFooter from './SignalFooter.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    ChevronLeftIcon,
    ChevronRightIcon,
    Delete02Icon,
  } from '@hugeicons/core-free-icons'

  let {
    page,
    pageSize,
    total,
    loading = false,
    onPage,
    onPageSize,
    onDelete,
    onDeleteAll,
  }: {
    page: number
    pageSize: number
    total: number
    loading?: boolean
    onPage: (page: number) => void
    onPageSize: (size: number) => void
    onDelete?: () => void
    onDeleteAll?: () => void
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

<SignalFooter
  index={page - 1}
  total={pages}
  label="log"
  onFirst={() => onPage(1)}
  onPrev={() => onPage(page - 1)}
  onNext={() => onPage(page + 1)}
  onLast={() => onPage(pages)}
  {onDelete}
>
  {#snippet leading()}
    <div class="pagination__summary">
      {#if onDeleteAll}<button
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
      <span role="status" aria-label="Log page range" class="tabular-nums"
        >{total === 0
          ? '0 logs'
          : `${((page - 1) * pageSize + 1).toLocaleString()}–${Math.min(page * pageSize, total).toLocaleString()} of ${total.toLocaleString()} logs`}</span
      >
    </div>
  {/snippet}
  {#snippet navigation()}
    <nav aria-label="Log pages" class="pagination__pages">
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
          class="btn btn-ghost btn-sm btn-square text-xs tabular-nums"
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
    </nav>
  {/snippet}
  {#snippet trailing()}
    <div class="pagination__size">
      <label class="flex items-center gap-2 text-xs text-base-content/60"
        >Rows
        <select
          class="select select-ghost select-xs w-16 text-xs"
          aria-label="Logs per page"
          value={pageSize}
          disabled={loading}
          onchange={event => onPageSize(Number(event.currentTarget.value))}
        >
          {#each [25, 50, 100, 250] as size}<option value={size}>{size}</option
            >{/each}
        </select>
      </label>
    </div>
  {/snippet}
</SignalFooter>

<style lang="postcss">
  @reference "../../app.css";
  .pagination__summary,
  .pagination__pages,
  .pagination__size {
    @apply flex min-w-0 items-center gap-1;
  }
  .pagination__pages {
    @apply justify-center;
  }
  .pagination__summary {
    @apply text-xs text-base-content/50;
  }
</style>
