<script module lang="ts">
  import {
    compareByOptionalBigintField,
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'
  import { traceSummaryDurationNs } from '@/utils/time'
  import type { TraceSummary } from '@/types/api-types'

  export type TraceSummarySortColumn =
    | 'serviceName'
    | 'rootSpanName'
    | 'startTime'
    | 'duration'
    | 'spanCount'
    | 'errorCount'

  export type TraceSummarySortDirection = 'asc' | 'desc'

  /** Primary key by column + direction; tie-break on trace ID. */
  function compareTraceSummaries(
    a: TraceSummary,
    b: TraceSummary,
    col: TraceSummarySortColumn,
    dir: TraceSummarySortDirection
  ): number {
    const cmp =
      col === 'serviceName'
        ? compareByStringField(a, b, t => t.rootSpan?.serviceName)
        : col === 'rootSpanName'
          ? compareByStringField(a, b, t => t.rootSpan?.name)
          : col === 'startTime'
            ? compareByTimestampField(a, b, t => t.startTime)
            : col === 'duration'
              ? compareByOptionalBigintField(a, b, traceSummaryDurationNs)
              : col === 'spanCount'
                ? a.spanCount - b.spanCount
                : a.errorCount - b.errorCount

    return cmp !== 0
      ? dir === 'asc'
        ? cmp
        : -cmp
      : a.traceID.localeCompare(b.traceID)
  }

  const SORT_OPTIONS = [
    { value: 'startTime', label: 'Start Time' },
    { value: 'duration', label: 'Duration' },
    { value: 'rootSpanName', label: 'Root Span Name' },
    { value: 'serviceName', label: 'Service Name' },
    { value: 'spanCount', label: 'Span Count' },
    { value: 'errorCount', label: 'Error Count' },
  ]
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import {
    signalIdFromPath,
    navigateToItem,
    getSpanFromQuery,
    setSpanInQuery,
  } from '@/utils/url-state'
  import { useRoute } from '@/state/route.svelte'
  import type {
    TraceData,
    SearchResultEvent,
    TraceStats,
  } from '@/types/api-types'
  import type { QueryNode } from '@/components/shared/Search/queryTree'
  import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import TraceCard from '@/components/traces/TraceCard.svelte'
  import DetailView from '@/components/traces/Detail/TraceDetailView.svelte'
  import WaterfallView from '@/components/traces/Waterfall/WaterfallView.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import { TrashIcon } from '@/icons'

  // --- context ---
  let timeContext = getTimeContext()

  // --- URL is the source of truth for the selected trace + span ---
  // `/traces/<traceID>?span=<spanID>`. Every selection change is a navigate (below).
  const route = useRoute()

  // --- state: API / list ---
  let traceSummaries = $state<TraceSummary[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)

  // --- state: sort ---
  let sortColumn = $state<TraceSummarySortColumn>('startTime')
  let sortDirection = $state<TraceSummarySortDirection>('desc')

  // --- selection + detail (selection derived from URL) ---
  let selectedTraceId = $derived(signalIdFromPath('traces', route.path))
  let selectedSpanID = $derived(route.query.span ?? null)
  let traceData = $state<TraceData | null>(null)
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

  let refreshAsideTip = $derived.by(() => {
    const parts: string[] = []
    if (pendingNewTraceCount > 0)
      parts.push(`+${pendingNewTraceCount.toLocaleString()} trace${pendingNewTraceCount !== 1 ? 's' : ''}`)
    if (pendingNewSpanCount > 0)
      parts.push(`+${pendingNewSpanCount.toLocaleString()} span${pendingNewSpanCount !== 1 ? 's' : ''}`)
    return parts.join(', ')
  })

  // --- effects ---

  let lastValidIndex = $state(0)

  // Auto-select a trace when none (or an out-of-range id) is selected. Guarded
  // behind mounted + !loading so a URL-provided id is never clobbered before
  // the list has finished fetching (shared-link load ordering).
  $effect(() => {
    if (!mounted || loading) return
    const id = selectedTraceId
    const idx = id ? sortedTraces.findIndex(t => t.traceID === id) : -1
    if (idx >= 0) {
      lastValidIndex = idx
    } else if (sortedTraces.length > 0) {
      const fallback =
        sortedTraces[Math.min(lastValidIndex, sortedTraces.length - 1)]
      if (fallback) navigateToItem('traces', fallback.traceID, { replace: true })
    } else if (id) {
      navigateToItem('traces', null, { replace: true })
    }
  })

  $effect(() => {
    const summary = selectedSummary
    if (!summary) {
      // Don't tear down the detail view while the list is still loading -- a
      // shared link's trace id may simply not be in the list yet.
      if (!mounted || loading) return
      traceData = null
      setSpanInQuery(null)
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
    // Explicit click is navigational: push so back returns to the prior trace.
    navigateToItem('traces', traceID, { replace: false })
  }

  // --- nav: walk sortedTraces ---

  let selectedIndex = $derived(
    selectedTraceId
      ? sortedTraces.findIndex(t => t.traceID === selectedTraceId)
      : -1
  )

  function selectByOffset(delta: number) {
    if (selectedIndex < 0 || sortedTraces.length === 0) return
    const target = Math.max(
      0,
      Math.min(sortedTraces.length - 1, selectedIndex + delta)
    )
    if (target === selectedIndex) return
    const next = sortedTraces[target]
    if (next) navigateToItem('traces', next.traceID, { replace: true })
  }

  function selectFirst() {
    const first = sortedTraces[0]
    if (first) navigateToItem('traces', first.traceID, { replace: true })
  }

  function selectLast() {
    const last = sortedTraces[sortedTraces.length - 1]
    if (last) navigateToItem('traces', last.traceID, { replace: true })
  }

  function handleSelectSpan(spanID: string) {
    // Clicking a span is navigational: push so back returns to the prior span.
    setSpanInQuery(spanID, { push: true })
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
      const spanIds = result.spans.map(n => n.spanData.spanID)
      const urlSpan = getSpanFromQuery()
      let desired: string | null
      if (queryTree) {
        const firstMatch = result.spans.find(n => n.matched)
        desired = firstMatch?.spanData.spanID ?? spanIds[0] ?? null
      } else if (urlSpan && spanIds.includes(urlSpan)) {
        // Honor a span carried by a shared/deep link.
        desired = urlSpan
      } else {
        desired = spanIds[0] ?? null
      }
      if (desired !== urlSpan) setSpanInQuery(desired)
    } catch (err) {
      console.error('Failed to fetch trace detail:', err)
      traceData = null
      setSpanInQuery(null)
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
      navigateToItem('traces', null, { replace: true })
      traceData = null
      await fetchTraces()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete traces'
      console.error('Error deleting traces:', err)
    }
  }

  async function handleDeleteTrace(traceID: string) {
    try {
      await telemetryAPI.deleteTraces([traceID])
      if (selectedTraceId === traceID) {
        navigateToItem('traces', null, { replace: true })
        traceData = null
      }
      await fetchTraces()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete trace'
      console.error('Error deleting trace:', err)
    }
  }

  // Wrapped form for the footer's onDelete: SignalFooter expects a
  // 0-arg callback. We bind the currently selected trace ID inside the
  // function (not at the prop site) so TS's narrowing on the truthy
  // check holds when the callback eventually fires.
  function deleteSelectedTrace() {
    if (traceData) handleDeleteTrace(traceData.traceID)
  }

  // --- lifecycle ---
  onMount(async () => {
    await fetchTraces()
    mounted = true
  })
</script>

<div class="traces-page">
  <PageLayout
    items={sortedTraces}
    selectedId={selectedTraceId}
    drawerId="signal-drawer"
    drawerLabel="Traces"
    onSelect={selectTrace}
    onRefresh={handleRefresh}
    {refreshPulse}
    {refreshAsideTip}
    {loading}
    itemKey={t => t.traceID}
    resizableStorageKey="trace-detail-panels"
    minDetailPx={352}
  >
    {#snippet drawerChromeToolbar()}
      <DrawerSearchPanel
        segment="toolbar"
        signal="traces"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
      />
    {/snippet}

    {#snippet drawerSearch()}
      <DrawerSearchPanel
        segment="search"
        signal="traces"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
        onSearchResults={handleSearchResults}
        onSearchReady={api => (searchEditorApi = api)}
      />
    {/snippet}

    {#snippet itemSnippet(trace, selected)}
      <TraceCard {trace} {selected} onclick={selectTrace} />
    {/snippet}

    {#snippet drawerFooter()}
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

    {#snippet main()}
      {#if error}
        <div class="traces-page__placeholder alert alert-error">
          <span>Error: {error}</span>
        </div>
      {:else if loading && !hasTraceRows}
        <div class="traces-page__placeholder traces-empty">
          Loading traces…
        </div>
      {:else if !loading && !hasTraceRows}
        <div class="traces-page__placeholder traces-empty">
          <p class="text-rp-subtle">No traces in this time range</p>
          <p class="mt-2 text-sm text-rp-muted">
            Send telemetry to the exporter or adjust the time range
          </p>
        </div>
      {:else if traceData}
        <WaterfallView
          spans={traceData.spans}
          {selectedSpanID}
          onSelectSpan={handleSelectSpan}
          loading={detailLoading}
        />
      {:else if detailLoading}
        <div class="traces-page__placeholder traces-empty">
          Loading trace detail…
        </div>
      {:else}
        <div class="traces-page__placeholder traces-empty">
          <p class="text-rp-subtle">Select a trace to view details</p>
        </div>
      {/if}
    {/snippet}

    {#snippet detail()}
      <DetailView span={selectedSpan} />
    {/snippet}

    {#snippet pageFooter()}
      <SignalFooter
        index={selectedIndex}
        total={sortedTraces.length}
        label="trace"
        onFirst={selectFirst}
        onPrev={() => selectByOffset(-1)}
        onNext={() => selectByOffset(1)}
        onLast={selectLast}
        onDelete={traceData ? deleteSelectedTrace : undefined}
      />
    {/snippet}
  </PageLayout>
</div>

<style lang="postcss">
  @reference "../app.css";

  .traces-page {
    @apply flex min-h-0 min-w-0 w-full flex-1;
  }

  .traces-page__placeholder {
    @apply m-[var(--layout-gap)];
  }

  .traces-empty {
    @apply px-4 py-12 text-center;
    color: var(--color-subtle);
  }
</style>
