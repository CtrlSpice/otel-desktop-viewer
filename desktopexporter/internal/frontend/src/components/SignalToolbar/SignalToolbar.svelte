<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { TraceDetailStats } from '@/utils/trace-detail-stats'
  import type { TraceListStats } from '@/components/TraceList/trace-list-stats'

  type TrailingFilterSlots = {
    /**
     * Ordered trailing filter UI: each snippet should render one control
     * (e.g. DateTimeFilter, FieldFilter). Rendered left-to-right after title/stats.
     */
    trailingFilters?: readonly Snippet[]
  }

  type SignalToolbarProps =
    | (TrailingFilterSlots & {
        signal: 'traces' | 'metrics' | 'logs'
        view: 'list'
        onRefresh?: (() => void) | null
        listStats?: TraceListStats | null
        /** Dim list summary stats (e.g. while refetching). */
        listStatsMuted?: boolean
      })
    | (TrailingFilterSlots & {
        signal: 'traces'
        view: 'detail'
        traceID: string
        traceStats?: TraceDetailStats | null
        onBack?: (() => void) | null
        onRefresh?: (() => void) | null
      })
    | (TrailingFilterSlots & {
        signal: 'metrics'
        view: 'detail'
        metricName: string
        onBack?: (() => void) | null
        onRefresh?: (() => void) | null
      })

  let props: SignalToolbarProps = $props()

  let signal = $derived(props.signal)
  let view = $derived(props.view)
  let onBack = $derived('onBack' in props ? (props.onBack ?? null) : null)
  let onRefresh = $derived(props.onRefresh ?? null)

  let detailTraceStats = $derived(
    signal === 'traces' && view === 'detail' && 'traceStats' in props
      ? (props.traceStats ?? null)
      : null
  )

  let traceDetailId = $derived(
    signal === 'traces' && view === 'detail' && 'traceID' in props
      ? props.traceID
      : null
  )

  let listStats = $derived(
    view === 'list' && 'listStats' in props ? (props.listStats ?? null) : null
  )

  let listStatsMuted = $derived(
    view === 'list' && 'listStatsMuted' in props
      ? Boolean(props.listStatsMuted)
      : false
  )

  let trailingFilters = $derived([...(props.trailingFilters ?? [])])
</script>

<div class="signal-toolbar">
  <div class="signal-toolbar__top-row">
    <!-- 1. Actions: back + refresh -->
    {#if onBack || onRefresh}
      <div class="signal-toolbar__action-group">
        {#if onBack}
          <button
            type="button"
            class="btn btn-ghost btn-sm btn-circle"
            onclick={onBack}
            aria-label="Go back"
          >
            <svg
              class="h-4 w-4"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="1.5"
            >
              <path d="M11 6h4.5a4.5 4.5 0 1 1 0 9H4" />
              <path d="M7 12s-3 2.21-3 3s3 3 3 3" />
            </svg>
          </button>
        {/if}
        {#if onRefresh}
          <button
            type="button"
            class="btn btn-soft btn-primary btn-sm btn-circle"
            onclick={onRefresh}
            aria-label="Refresh"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24">
              <path
                d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
              />
            </svg>
          </button>
        {/if}
      </div>
    {/if}

    <!-- 2. Title + Stats -->
    {#if listStats}
      {@const s = listStats}
      <div
        class="signal-toolbar__stats-scroll"
        class:signal-toolbar__stats-scroll--muted={listStatsMuted}
        aria-label="List summary"
        aria-busy={listStatsMuted}
      >
        <div class="signal-toolbar__stats-inner text-sm text-base-content/70">
          <span class="shrink-0 font-medium text-base-content"
            >{s.traces} trace{s.traces !== 1 ? 's' : ''}</span
          >
          <span class="text-base-content/35" aria-hidden="true">·</span>
          <span class="shrink-0">{s.spans} span{s.spans !== 1 ? 's' : ''}</span>
          <span class="text-base-content/35" aria-hidden="true">·</span>
          <span class="shrink-0"
            >{s.services} service{s.services !== 1 ? 's' : ''}</span
          >
          <span class="text-base-content/35" aria-hidden="true">·</span>
          <span class="shrink-0 {s.errors > 0 ? 'text-error/90' : ''}"
            >{s.errors} error{s.errors !== 1 ? 's' : ''}</span
          >
          <span class="text-base-content/35" aria-hidden="true">·</span>
          <span class="shrink-0 {s.exceptions > 0 ? 'text-warning/90' : ''}"
            >{s.exceptions} exception{s.exceptions !== 1 ? 's' : ''}</span
          >
        </div>
      </div>
    {:else if view === 'detail' && traceDetailId}
      <div class="signal-toolbar__title">
        <span class="signal-toolbar__detail-title" title={traceDetailId}
          >{traceDetailId}</span
        >
      </div>
      {#if detailTraceStats}
        {@const s = detailTraceStats}
        <span class="text-base-content/35" aria-hidden="true">·</span>
        <div class="signal-toolbar__stats-scroll" aria-label="Trace summary">
          <div class="signal-toolbar__stats-inner text-sm text-base-content/70">
            <span class="shrink-0"
              >{s.spanCount} span{s.spanCount !== 1 ? 's' : ''}</span
            >
            <span class="text-base-content/35" aria-hidden="true">·</span>
            <span class="shrink-0"
              >{s.serviceCount} service{s.serviceCount !== 1 ? 's' : ''}</span
            >
            <span class="text-base-content/35" aria-hidden="true">·</span>
            <span class="shrink-0 {s.errorCount > 0 ? 'text-error/90' : ''}"
              >{s.errorCount} error{s.errorCount !== 1 ? 's' : ''}</span
            >
            <span class="text-base-content/35" aria-hidden="true">·</span>
            <span
              class="shrink-0 {s.exceptionCount > 0 ? 'text-warning/90' : ''}"
              >{s.exceptionCount} exception{s.exceptionCount !== 1
                ? 's'
                : ''}</span
            >
          </div>
        </div>
      {/if}
    {:else if view === 'detail' && 'metricName' in props}
      <div class="signal-toolbar__title">
        <span class="signal-toolbar__detail-title" title={props.metricName}
          >{props.metricName}</span
        >
      </div>
    {:else}
      <div class="signal-toolbar__title">
        <span class="signal-toolbar__list-title"
          >{signal.charAt(0).toUpperCase() + signal.slice(1)}</span
        >
      </div>
    {/if}

    <!-- 3. Trailing filters (page-defined order) -->
    {#if trailingFilters.length > 0}
      <div
        class="signal-toolbar__trailing-filters"
        aria-label="Toolbar filters"
      >
        {#each trailingFilters as render, i (i)}
          <div class="signal-toolbar__trailing-item">
            {@render render()}
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style lang="postcss">
  @reference "../../app.css";

  .signal-toolbar {
    @apply relative w-full min-w-0;
  }

  .signal-toolbar__top-row {
    @apply flex w-full min-w-0 flex-nowrap items-center px-2 py-1;
    gap: var(--layout-gap);
    min-height: var(--toolbar-search-chrome-min-height);
    box-sizing: border-box;
  }

  /* No outline rings on toolbar actions / filters (soft + primary still read from fill). */
  .signal-toolbar__top-row :global(.btn) {
    @apply border-0 shadow-none;
  }

  .signal-toolbar__top-row :global(.toolbar-filter-trigger) {
    @apply border-0;
  }

  .signal-toolbar__top-row :global(.toolbar-filter-trigger__dropdown-circle) {
    @apply border-0;
  }

  .signal-toolbar__action-group {
    @apply flex shrink-0 flex-nowrap items-center gap-1.5 mr-1;
  }

  .signal-toolbar__title {
    @apply flex shrink-0 items-center;
  }

  .signal-toolbar__detail-title {
    @apply max-w-[min(100%,20rem)] truncate text-sm font-medium text-base-content sm:max-w-[28rem];
  }

  .signal-toolbar__list-title {
    @apply text-sm font-semibold text-base-content;
  }

  .signal-toolbar__trailing-filters {
    @apply ml-auto flex min-w-0 max-w-full shrink flex-nowrap items-center justify-end gap-2;
  }

  .signal-toolbar__trailing-item {
    @apply flex min-w-0 max-w-full shrink items-center;
  }

  .signal-toolbar__stats-scroll {
    @apply min-w-0 flex-1 overflow-x-auto transition-opacity duration-300 ease-in-out;
    scrollbar-width: thin;
  }

  .signal-toolbar__stats-scroll--muted {
    @apply opacity-[0.55];
  }

  .signal-toolbar__stats-inner {
    @apply inline-flex min-w-max flex-nowrap items-center gap-x-2 py-0.5;
  }
</style>
