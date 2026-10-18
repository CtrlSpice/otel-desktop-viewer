<script lang="ts">
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { RefreshIcon } from '@hugeicons/core-free-icons'
  import DateTimeFilter from '../Toolbar/DateTimeFilter.svelte'

  let {
    search,
    controls,
    onRefresh,
    refreshPulse = false,
    refreshAsideTip = '',
  }: {
    search?: Snippet
    controls?: Snippet
    onRefresh?: () => void
    refreshPulse?: boolean
    refreshAsideTip?: string
  } = $props()

  const refreshLabel = $derived(
    refreshPulse && refreshAsideTip ? `Refresh — ${refreshAsideTip}` : 'Refresh'
  )
</script>

<div
  class="search-bar flex w-full min-w-0 items-center gap-1 rounded-full text-sm text-base-content {search
    ? 'bg-base-300 px-1.5'
    : ''}"
  role={search ? 'search' : undefined}
  aria-label={search ? 'Workspace search' : undefined}
>
  {#if onRefresh || search}
    <div class="shrink-0 rounded-full {search ? 'bg-base-300' : ''}">
      <button
        type="button"
        class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content tooltip tooltip-bottom"
        aria-label={refreshLabel}
        data-tip={refreshLabel}
        disabled={!onRefresh}
        onclick={onRefresh}
      >
        <HugeiconsIcon
          icon={RefreshIcon}
          size={17}
          strokeWidth={1.5}
          aria-hidden="true"
        />
      </button>
    </div>
    {#if refreshPulse}<span class="badge-count text-sm!" role="status"
        >{refreshAsideTip || 'New data'}</span
      >{/if}
  {/if}
  <div
    class="search-bar__query flex min-w-0 flex-1 items-center rounded-full {search
      ? 'bg-base-300'
      : ''}"
  >
    <div
      class="search-bar__field min-w-0 flex-1"
      style="--typed-field-bg: transparent"
    >
      {#if search}{@render search()}{/if}
    </div>
  </div>
  <div
    class="flex shrink-0 items-center rounded-full {search
      ? 'bg-base-300'
      : ''}"
  >
    {#if controls}{@render controls()}{:else}<DateTimeFilter />{/if}
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .search-bar__query {
    height: 2rem;
  }

  .search-bar__field {
    display: flex;
    align-items: center;
    width: 0;
    height: 2rem;
    overflow: hidden;
  }

  .search-bar__query:has(
      :global(.search-editor-container--error)
    ):focus-within {
    outline: var(--focus-ring-width) solid var(--color-error);
    outline-offset: var(--focus-ring-offset);
  }

  .search-bar__field
    :global(.typed-field:not(.search-editor-container--error)) {
    border-color: transparent;
  }

  .search-bar__query .search-bar__field :global(.typed-field) {
    padding-left: 0.5rem;
  }

  .search-bar__field :global(.typed-field::placeholder),
  .search-bar__field :global(.cm-placeholder) {
    color: var(--color-muted);
  }

  .search-bar__field :global(.typed-field:focus-visible),
  .search-bar__field :global(.search-editor-container:focus-within) {
    outline: none;
  }
</style>
