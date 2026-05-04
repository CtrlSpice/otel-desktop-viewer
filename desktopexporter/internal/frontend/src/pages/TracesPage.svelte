<script module lang="ts">
  import {
    compareTraceSummaries,
    type TraceSummarySortColumn,
    type TraceSummarySortDirection,
  } from '@/utils/traces'

  const SORT_OPTIONS = [
    { value: 'startTime', label: 'Start Time' },
    { value: 'duration', label: 'Duration' },
    { value: 'rootSpanName', label: 'Root Span' },
    { value: 'serviceName', label: 'Service' },
    { value: 'spanCount', label: 'Spans' },
  ]
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import type {
    TraceSummary,
    TraceData,
    SearchResultEvent,
    TraceStats,
  } from '@/types/api-types'
  import type { QueryNode } from '@/components/SignalToolbar/search/queryTree'
  import type { SearchEditorAPI } from '@/components/SignalToolbar/search/search-editor-api'
  import SignalListDrawer from '@/components/SignalListDrawer.svelte'
  import DrawerSearchPanel from '@/components/DrawerSearchPanel.svelte'
  import TraceCard from '@/components/TraceCard.svelte'
  import DetailView from '@/components/TraceDetails/DetailView/DetailView.svelte'
  import WaterfallView from '@/components/TraceDetails/Waterfall/WaterfallView.svelte'
  import ResizablePanels from '@/components/ResizablePanels.svelte'
  import { TrashIcon } from '@/icons'

  // --- context ---
  let timeContext = getTimeContext()

  // --- state: API / list ---
  let traceSummaries = $state<TraceSummary[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)

  // --- state: sort ---
  let sortColumn = $state<TraceSummarySortColumn>('startTime')
  let sortDirection = $state<TraceSummarySortDirection>('desc')

  // --- state: selection + detail ---
  let selectedTraceId = $state<string | null>(null)
  let traceData = $state<TraceData | null>(null)
  let selectedSpanID = $state<string | null>(null)
  let detailLoading = $state(false)

  // --- state: active search query (for span highlighting in waterfall) ---
  let activeQueryTree = $state<QueryNode | undefined>(undefined)

  // --- state: polling / refresh ---
  let searchEditorApi = $state<SearchEditorAPI | null>(null)
  let baselineStats = $state<TraceStats | null>(null)
  let polledStats = $state<TraceStats | null>(null)
  const POLL_INTERVAL_MS = 3000

  // --- derived ---
  let sortedTraces = $derived.by(() => {
    const col = sortColumn
    const dir = sortDirection
    const rows = [...traceSummaries]
    rows.sort((a, b) => compareTraceSummaries(a, b, col, dir))
    return rows
  })

  let hasTraceRows = $derived(traceSummaries.length > 0)

  let selectedSummary = $derived(
    selectedTraceId
      ? sortedTraces.find(t => t.traceID === selectedTraceId)
      : undefined
  )

  let selectedSpan = $derived(
    traceData?.spans.find(n => n.spanData.spanID === selectedSpanID)
      ?.spanData ??
      traceData?.spans[0]?.spanData ??
      undefined
  )

  let pendingNewTraceCount = $derived.by(() => {
    if (!baselineStats || !polledStats) return 0
    const delta = polledStats.traceCount - baselineStats.traceCount
    return delta > 0 ? delta : 0
  })

  let pendingNewSpanCount = $derived.by(() => {
    if (!baselineStats || !polledStats) return 0
    const delta = polledStats.spanCount - baselineStats.spanCount
    return delta > 0 ? delta : 0
  })

  let refreshPulse = $derived(
    pendingNewTraceCount > 0 || pendingNewSpanCount > 0
  )

  // --- effects ---

  $effect(() => {
    if (
      !selectedTraceId ||
      !sortedTraces.some(t => t.traceID === selectedTraceId)
    ) {
      const first = sortedTraces[0]
      selectedTraceId = first?.traceID ?? null
    }
  })

  $effect(() => {
    const summary = selectedSummary
    if (!summary) {
      traceData = null
      selectedSpanID = null
      return
    }
    fetchTraceDetail(summary.traceID, activeQueryTree)
  })

  $effect(() => {
    void timeContext.selection
    if (mounted) {
      fetchTraces()
    }
  })

  $effect(() => {
    if (!mounted) return
    const id = setInterval(async () => {
      try {
        const s = await telemetryAPI.getStats()
        polledStats = s.traces
      } catch {
        /* polling failures are silent */
      }
    }, POLL_INTERVAL_MS)
    return () => clearInterval(id)
  })

  // --- handlers ---
  function handleSortChange(value: string, direction: 'asc' | 'desc') {
    sortColumn = value as TraceSummarySortColumn
    sortDirection = direction
  }

  function selectTrace(traceID: string) {
    selectedTraceId = traceID
  }

  function handleSelectSpan(spanID: string) {
    selectedSpanID = spanID
  }

  async function fetchTraces() {
    try {
      loading = true
      error = null
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      traceSummaries = await telemetryAPI.searchTraces(startTime, endTime)
      const s = await telemetryAPI.getStats()
      baselineStats = s.traces
      polledStats = s.traces
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to fetch traces'
      console.error('Error fetching trace summaries:', err)
    } finally {
      loading = false
    }
  }

  async function fetchTraceDetail(traceID: string, queryTree?: QueryNode) {
    try {
      detailLoading = true
      const result = await telemetryAPI.searchSpans(traceID, queryTree)
      traceData = result
      if (queryTree) {
        const firstMatch = result.spans.find(n => n.matched)
        selectedSpanID =
          firstMatch?.spanData.spanID ??
          result.spans[0]?.spanData.spanID ??
          null
      } else {
        selectedSpanID = result.spans[0]?.spanData.spanID ?? null
      }
    } catch (err) {
      console.error('Failed to fetch trace detail:', err)
      traceData = null
      selectedSpanID = null
    } finally {
      detailLoading = false
    }
  }

  function handleRefresh() {
    searchEditorApi?.clear()
    fetchTraces()
  }

  function handleSearchResults(event: SearchResultEvent) {
    if (event.signal === 'traces') {
      loading = false
      error = null
      traceSummaries = event.results
      activeQueryTree = event.queryTree as QueryNode | undefined
    }
  }

  async function handleDeleteAllTraces() {
    try {
      await telemetryAPI.clearTraces()
      selectedTraceId = null
      traceData = null
      selectedSpanID = null
      await fetchTraces()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete traces'
      console.error('Error deleting traces:', err)
    }
  }

  // --- lifecycle ---
  onMount(async () => {
    await fetchTraces()
    mounted = true
  })
</script>

<div class="traces-page">
  <SignalListDrawer
    items={sortedTraces}
    selectedId={selectedTraceId}
    drawerId="signal-drawer"
    label="Traces"
    count={sortedTraces.length}
    storageKey="trace-drawer"
    onSelect={selectTrace}
    onRefresh={handleRefresh}
    {refreshPulse}
    itemKey={t => t.traceID}
  >
    {#snippet refreshAside()}
      {#if pendingNewTraceCount > 0}
        <span class="signal-drawer__refresh-aside-pill">
          +{pendingNewTraceCount.toLocaleString()}
          trace{pendingNewTraceCount !== 1 ? 's' : ''}
        </span>
      {/if}
      {#if pendingNewSpanCount > 0}
        <span class="signal-drawer__refresh-aside-pill">
          +{pendingNewSpanCount.toLocaleString()}
          span{pendingNewSpanCount !== 1 ? 's' : ''}
        </span>
      {/if}
    {/snippet}

    {#snippet drawerChrome()}
      <DrawerSearchPanel
        segment="chrome"
        signal="traces"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
      />
    {/snippet}

    {#snippet drawerChromeToolbar()}
      <DrawerSearchPanel
        segment="toolbar"
        signal="traces"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
        onRefresh={handleRefresh}
        {refreshPulse}
      >
        {#snippet refreshAside()}
          {#if pendingNewTraceCount > 0}
            <span class="signal-drawer__refresh-aside-pill">
              +{pendingNewTraceCount.toLocaleString()}
              trace{pendingNewTraceCount !== 1 ? 's' : ''}
            </span>
          {/if}
          {#if pendingNewSpanCount > 0}
            <span class="signal-drawer__refresh-aside-pill">
              +{pendingNewSpanCount.toLocaleString()}
              span{pendingNewSpanCount !== 1 ? 's' : ''}
            </span>
          {/if}
        {/snippet}
      </DrawerSearchPanel>
    {/snippet}

    {#snippet drawerSearch()}
      <DrawerSearchPanel
        segment="search"
        signal="traces"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
        onRefresh={handleRefresh}
        {refreshPulse}
        onSearchResults={handleSearchResults}
        onSearchReady={api => (searchEditorApi = api)}
      />
    {/snippet}

    {#snippet itemSnippet(trace, selected)}
      <TraceCard {trace} {selected} onclick={selectTrace} />
    {/snippet}

    {#snippet footer()}
      <div class="flex items-center justify-between">
        <span class="text-xs tabular-nums text-base-content/50">
          {sortedTraces.length} trace{sortedTraces.length !== 1 ? 's' : ''}
        </span>
        <button
          type="button"
          class="btn btn-ghost btn-xs text-error"
          onclick={handleDeleteAllTraces}
          aria-label="Delete all traces"
        >
          <TrashIcon class="h-3 w-3" aria-hidden="true" />
          Delete all
        </button>
      </div>
    {/snippet}

    {#snippet children()}
      <div class="traces-content">
        <div class="traces-content__body">
          {#if error}
            <div class="alert alert-error">
              <span>Error: {error}</span>
            </div>
          {:else if loading && !hasTraceRows}
            <div class="traces-empty">Loading traces…</div>
          {:else if !loading && !hasTraceRows}
            <div class="traces-empty">
              <p class="text-base-content/60">No traces in this time range</p>
              <p class="mt-2 text-sm text-base-content/50">
                Send telemetry to the exporter or adjust the time range
              </p>
            </div>
          {:else if traceData}
            {@const data = traceData}
            <div class="traces-detail">
              <ResizablePanels
                defaultLeftWidth={0.7}
                minLeftWidth={0.3}
                minRightWidth={0.2}
                storageKey="trace-detail-panels"
              >
                {#snippet leftPanel()}
                  <WaterfallView
                    spans={data.spans}
                    {selectedSpanID}
                    onSelectSpan={handleSelectSpan}
                    loading={detailLoading}
                  />
                {/snippet}
                {#snippet rightPanel()}
                  <div
                    class="h-full overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm"
                  >
                    <DetailView span={selectedSpan} />
                  </div>
                {/snippet}
              </ResizablePanels>
            </div>
          {:else if detailLoading}
            <div class="traces-empty">Loading trace detail…</div>
          {:else}
            <div class="traces-empty">
              <p class="text-base-content/60">Select a trace to view details</p>
            </div>
          {/if}
        </div>
      </div>
    {/snippet}
  </SignalListDrawer>
</div>

<style lang="postcss">
  @reference "../app.css";

  .traces-page {
    @apply flex min-h-0 min-w-0 w-full flex-1;
  }

  .traces-content {
    @apply relative flex min-h-0 min-w-0 flex-1 flex-col;
  }

  .traces-content__body {
    @apply flex min-h-0 min-w-0 flex-1 flex-col p-[var(--layout-gap)];
  }

  .traces-detail {
    @apply flex-1 min-h-0 min-w-0;
  }

  .traces-empty {
    @apply rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center text-base-content/60 shadow-surface-sm backdrop-blur-sm;
  }
</style>
