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
  import { telemetryAPI, isAbortError } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import {
    navigateToItem,
    getSpanFromQuery,
    setSpanInQuery,
    selectSpanEvent,
    setEventInQuery,
    SPAN_PARAM,
    EVENT_PARAM,
  } from '@/route'
  import type {
    TraceData,
    SearchResultEvent,
    TraceStats,
  } from '@/types/api-types'
  import type { QueryNode } from '@/components/shared/Search/queryTree'
  import { createSignalListPage } from '@/contexts/signal-list-page.svelte'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import SignalDrawerFooter from '@/components/shared/Drawer/SignalDrawerFooter.svelte'
  import TraceCard from '@/components/traces/TraceCard.svelte'
  import DetailView from '@/components/traces/Detail/TraceDetailView.svelte'
  import WaterfallView from '@/components/traces/Waterfall/WaterfallView.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'

  let timeContext = getTimeContext()
  const routeContext = getRouteContext()

  let baselineStats = $state<TraceStats | null>(null)
  let polledStats = $state<TraceStats | null>(null)
  let actionError = $state<string | null>(null)

  const page = createSignalListPage<TraceSummary>({
    signal: 'traces',
    getItemId: trace => trace.traceID,
    initialSort: { column: 'startTime', direction: 'desc' },
    compare: (a, b, col, dir) =>
      compareTraceSummaries(
        a,
        b,
        col as TraceSummarySortColumn,
        dir as TraceSummarySortDirection
      ),
    fetchList: async () => {
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      const results = await telemetryAPI.searchTraces(startTime, endTime)
      const s = await telemetryAPI.getStats()
      baselineStats = s.traces
      polledStats = s.traces
      return results
    },
    pollStats: async () => {
      const s = await telemetryAPI.getStats()
      polledStats = s.traces
    },
    refreshFromStats: () => {
      if (!baselineStats || !polledStats) {
        return { pulse: false, tip: '' }
      }
      const parts: string[] = []
      const traceDelta = polledStats.traceCount - baselineStats.traceCount
      if (traceDelta > 0) {
        parts.push(
          `+${traceDelta.toLocaleString()} trace${traceDelta !== 1 ? 's' : ''}`
        )
      }
      const spanDelta = polledStats.spanCount - baselineStats.spanCount
      if (spanDelta > 0) {
        parts.push(
          `+${spanDelta.toLocaleString()} span${spanDelta !== 1 ? 's' : ''}`
        )
      }
      return { pulse: parts.length > 0, tip: parts.join(', ') }
    },
  })

  // `/traces/<traceID>?span=<spanID>&event=<index>` — span/event in query string.
  let selectedSpanID = $derived(routeContext.route.query[SPAN_PARAM] ?? null)
  let selectedEventIndex = $derived.by((): number | null => {
    const raw = routeContext.route.query[EVENT_PARAM]
    if (!raw) return null
    const index = Number.parseInt(raw, 10)
    if (!Number.isFinite(index) || index < 0) return null
    return index
  })
  let traceData = $state<TraceData | null>(null)
  let detailLoading = $state(false)
  let activeQueryTree = $state<QueryNode | undefined>(undefined)

  let hasTraceRows = $derived(page.items.length > 0)
  let displayError = $derived(page.error ?? actionError)

  let selectedSpan = $derived(
    traceData?.spans.find(n => n.spanData.spanID === selectedSpanID)
      ?.spanData ??
      traceData?.spans[0]?.spanData ??
      undefined
  )

  let resolvedEventIndex = $derived.by((): number | null => {
    const span = selectedSpan
    const index = selectedEventIndex
    if (index === null || !span) return null
    if (index >= span.events.length) return null
    return index
  })

  $effect(() => {
    const index = selectedEventIndex
    const span = selectedSpan
    if (index === null) return
    if (!span || index >= span.events.length) setEventInQuery(null)
  })

  $effect(() => {
    const summary = page.selectedSummary
    if (!summary) {
      // Don't tear down the detail view while the list is still loading -- a
      // shared link's trace id may simply not be in the list yet.
      if (!page.mounted || page.loading) return
      traceData = null
      setSpanInQuery(null)
      return
    }
    fetchTraceDetail(summary.traceID, activeQueryTree)
  })

  function selectTrace(traceID: string) {
    page.selectItem(traceID)
  }

  function handleSelectSpan(spanID: string) {
    setSpanInQuery(spanID, 'push')
  }

  function handleSelectEvent(spanID: string, eventIndex: number) {
    selectSpanEvent(spanID, eventIndex, 'push')
  }

  function handleSearchResults(event: SearchResultEvent) {
    page.handleSearchResults(event)
    if (event.signal === 'traces') {
      activeQueryTree = event.queryTree as QueryNode | undefined
    }
  }

  // Each fetch supersedes the last: clicking down a trace list abandons the
  // previous searchSpans, and without aborting it the query keeps running
  // server-side holding the store's read lock for a result we have already
  // thrown away.
  let detailFetch: AbortController | null = null

  async function fetchTraceDetail(traceID: string, queryTree?: QueryNode) {
    detailFetch?.abort()
    const fetchCtl = new AbortController()
    detailFetch = fetchCtl

    try {
      detailLoading = true
      const result = await telemetryAPI.searchSpans(
        traceID,
        queryTree,
        fetchCtl.signal
      )
      traceData = result
      const spanIds = result.spans.map(n => n.spanData.spanID)
      const urlSpan = getSpanFromQuery()
      let desired: string | null
      if (queryTree) {
        const firstMatch = result.spans.find(n => n.matched)
        desired = firstMatch?.spanData.spanID ?? spanIds[0] ?? null
      } else if (urlSpan && spanIds.includes(urlSpan)) {
        desired = urlSpan
      } else {
        desired = spanIds[0] ?? null
      }
      if (desired !== urlSpan) setSpanInQuery(desired)
    } catch (err) {
      // A superseded fetch is not a failure: a newer one is already in flight
      // and owns the view state.
      if (isAbortError(err)) return
      console.error('Failed to fetch trace detail:', err)
      traceData = null
      setSpanInQuery(null)
    } finally {
      // Only the newest fetch owns the loading flag; an aborted older one
      // must not clear it out from under its replacement.
      if (detailFetch === fetchCtl) {
        detailLoading = false
        detailFetch = null
      }
    }
  }

  async function handleDeleteAllTraces() {
    actionError = null
    try {
      await telemetryAPI.clearTraces()
      navigateToItem('traces', null, 'replace')
      traceData = null
      await page.runListFetch()
    } catch (err) {
      actionError =
        err instanceof Error ? err.message : 'Failed to delete traces'
      console.error('Error deleting traces:', err)
    }
  }

  async function handleDeleteTrace(traceID: string) {
    actionError = null
    try {
      await telemetryAPI.deleteTraces([traceID])
      if (page.selectedId === traceID) {
        navigateToItem('traces', null, 'replace')
        traceData = null
      }
      await page.runListFetch()
    } catch (err) {
      actionError =
        err instanceof Error ? err.message : 'Failed to delete trace'
      console.error('Error deleting trace:', err)
    }
  }

  function deleteSelectedTrace() {
    if (traceData) handleDeleteTrace(traceData.traceID)
  }
</script>

<div class="traces-page">
  <PageLayout
    items={page.sortedItems}
    selectedId={page.selectedId}
    drawerId="signal-drawer"
    drawerLabel="Traces"
    onRefresh={page.handleRefresh}
    refreshPulse={page.refreshPulse}
    refreshAsideTip={page.refreshAsideTip}
    loading={page.loading}
    itemKey={t => t.traceID}
    resizableStorageKey="trace-detail-panels"
    minDetailPx={352}
  >
    {#snippet drawerChromeToolbar()}
      <DrawerSearchPanel
        segment="toolbar"
        signal="traces"
        sortOptions={SORT_OPTIONS}
        sortValue={page.sortColumn}
        sortDirection={page.sortDirection}
        onSortChange={page.handleSortChange}
      />
    {/snippet}

    {#snippet drawerSearch()}
      <DrawerSearchPanel
        segment="search"
        signal="traces"
        sortOptions={SORT_OPTIONS}
        sortValue={page.sortColumn}
        sortDirection={page.sortDirection}
        onSortChange={page.handleSortChange}
        onSearchResults={handleSearchResults}
        onSearchReady={api => (page.searchEditorApi = api)}
      />
    {/snippet}

    {#snippet itemSnippet(trace, selected)}
      <TraceCard {trace} {selected} onclick={selectTrace} />
    {/snippet}

    {#snippet drawerFooter()}
      <SignalDrawerFooter
        count={page.sortedItems.length}
        label="trace"
        onDeleteAll={handleDeleteAllTraces}
      />
    {/snippet}

    {#snippet main()}
      {#if displayError}
        <div class="traces-page__placeholder alert alert-error">
          <span>Error: {displayError}</span>
        </div>
      {:else if page.loading && !hasTraceRows}
        <div class="traces-page__placeholder traces-empty">
          Loading traces…
        </div>
      {:else if !page.loading && !hasTraceRows}
        <div class="traces-page__placeholder traces-empty">
          <p class="text-rp-subtle">No traces in this time range</p>
          <p class="mt-2 text-sm text-rp-muted">
            Send telemetry to the exporter or adjust the time range
          </p>
        </div>
      {:else if traceData}
        {#if traceData.unplacedSpanCount > 0}
          <div class="traces-page__unplaced alert alert-warning" role="alert">
            <span>
              {traceData.unplacedSpanCount}
              {traceData.unplacedSpanCount === 1 ? 'span is' : 'spans are'} missing
              from this trace. Their parent links form a loop, so they have no
              place in the tree — usually an instrumentation bug in the service
              that emitted them.
            </span>
          </div>
        {/if}
        <WaterfallView
          spans={traceData.spans}
          {selectedSpanID}
          onSelectSpan={handleSelectSpan}
          onSelectEvent={handleSelectEvent}
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
      <DetailView span={selectedSpan} selectedEventIndex={resolvedEventIndex} />
    {/snippet}

    {#snippet pageFooter()}
      <SignalFooter
        index={page.selectedIndex}
        total={page.sortedItems.length}
        label="trace"
        onFirst={page.selectFirst}
        onPrev={() => page.selectByOffset(-1)}
        onNext={() => page.selectByOffset(1)}
        onLast={page.selectLast}
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

  /* Sits above the waterfall rather than replacing it: the spans that could
     be placed are still worth looking at, and the notice explains why the
     trace is short. */
  .traces-page__unplaced {
    @apply mx-[var(--layout-gap)] mt-[var(--layout-gap)] mb-0 text-sm;
  }

  .traces-empty {
    @apply px-4 py-12 text-center;
    color: var(--color-subtle);
  }
</style>
