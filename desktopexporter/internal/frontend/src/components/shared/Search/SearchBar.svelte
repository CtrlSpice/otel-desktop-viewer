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
    refreshPulse && refreshAsideTip ? `Refresh: ${refreshAsideTip}` : 'Refresh'
  )
</script>

<div
  class="search-bar flex w-full min-w-0 items-center rounded-full text-sm text-base-content {search
    ? 'search-bar--editable bg-(--color-input)'
    : ''}"
  role={search ? 'search' : undefined}
  aria-label={search ? 'Workspace search' : undefined}
>
  {#if onRefresh || search}
    <div
      class="shrink-0 rounded-full {refreshPulse
        ? 'aura aura-xs search-bar__freshness text-primary'
        : ''}"
    >
      <button
        type="button"
        class="search-bar__refresh btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content tooltip tooltip-bottom"
        class:search-bar__refresh--fresh={refreshPulse}
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
    {#if refreshPulse}
      <span class="sr-only" role="status">{refreshAsideTip || 'New data'}</span>
    {/if}
  {/if}
  <div class="search-bar__query flex min-w-0 flex-1 items-center rounded-full">
    <div
      class="search-bar__field min-w-0 flex-1"
      style="--typed-field-bg: transparent"
    >
      {#if search}{@render search()}{/if}
    </div>
  </div>
  <div class="flex shrink-0 items-center rounded-full">
    {#if controls}{@render controls()}{:else}<DateTimeFilter />{/if}
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .search-bar {
    gap: var(--control-icon-gap);
  }

  .search-bar--editable {
    border: 1px solid var(--color-border);
    padding-inline: 0.25rem;
  }

  .search-bar--editable:has(.search-bar__field :global(:focus)) {
    outline: var(--focus-ring-width) solid var(--focus-ring-color);
    outline-offset: var(--focus-ring-offset);
  }

  .search-bar--editable:has(:global(.search-editor-container--error)) {
    border-color: var(--color-error);
    outline-color: var(--color-error);
  }

  .search-bar__refresh--fresh {
    background-color: var(--color-input);
  }

  .search-bar__freshness {
    --aura-radius: 9999px;
    line-height: 0;
  }

  .search-bar__freshness::before {
    filter: blur(2px);
    opacity: 0.35;
    pointer-events: none;
  }

  .search-bar__freshness::after {
    filter: blur(4px);
    opacity: 0.2;
    pointer-events: none;
  }

  @media (prefers-reduced-motion: reduce) {
    .search-bar__freshness {
      animation: none;
    }
  }

  .search-bar__query {
    height: 2rem;
  }

  .search-bar__field {
    --search-editor-row-height: 2rem;
    display: flex;
    align-items: center;
    width: 0;
    height: 2rem;
    overflow: visible;
  }

  /* Keep the bubble inside the viewport when search spans the mobile header. */
  .search-bar__refresh::before {
    left: 0;
    --tt-trans: 0%;
  }

  .search-bar :global(.tooltip[data-tip]::before) {
    max-width: min(20rem, calc(100vw - 2rem));
  }

  .search-bar__query .search-bar__field :global(.typed-field) {
    border: 0;
    height: 2rem;
    min-height: 2rem;
    padding-block: 0;
    padding-left: 0.5rem;
  }

  .search-bar__field :global(.typed-field::placeholder),
  .search-bar__field :global(.cm-placeholder) {
    color: var(--color-subtle);
  }

  .search-bar__field :global(.typed-field:focus-visible),
  .search-bar__field :global(.search-editor-container:focus-within) {
    outline: none;
  }
</style>
