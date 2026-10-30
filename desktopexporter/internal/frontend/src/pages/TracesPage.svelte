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
</script>

<script lang="ts">
  import { onDestroy, untrack } from 'svelte'
  import { telemetryAPI, isAbortError } from '@/services/telemetry-service'
  import { confirmDeleteAll } from '@/utils/confirm-delete'
  import {
    getTimeContext,
    selectionToQueryRangeNs,
  } from '@/contexts/time-context.svelte'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import {
    navigateToItem,
    getSpanFromQuery,
    setSpanInQuery,
    setEventInQuery,
    setLogInQuery,
    selectSpanEvent,
    selectSpanLog,
    parseEventIndex,
    SPAN_PARAM,
    EVENT_PARAM,
    LOG_PARAM,
  } from '@/route'
  import type {
    TraceWaterfallData,
    SpanData,
    TraceColumnPreview,
    SearchResultEvent,
    TraceStats,
    TraceLogSummary,
  } from '@/types/api-types'
  import type {
    FieldDefinition,
    ParsedSearchRequest,
    QueryNode,
  } from '@/search/model'
  import { parseSearchRequest } from '@/components/shared/Search/queryParser'
  import type { TimelineRecord } from '@/components/traces/Waterfall/timeline-markers'
  import { createSignalListPage } from '@/contexts/signal-list-page.svelte'

  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import DrawerSortControl from '@/components/shared/Drawer/DrawerSortControl.svelte'
  import ListHeaderControls from '@/components/shared/ListHeaderControls.svelte'
  import TelemetryListFilters from '@/components/shared/TelemetryListFilters.svelte'
  import SignalDrawerFooter from '@/components/shared/Drawer/SignalDrawerFooter.svelte'
  import TraceCard from '@/components/traces/TraceCard.svelte'
  import DetailView from '@/components/traces/Detail/TraceDetailView.svelte'
  import WaterfallView from '@/components/traces/Waterfall/WaterfallView.svelte'
  import type { WaterfallColumn } from '@/components/traces/Waterfall/waterfall-columns'
  import type { DetailFieldTarget } from '@/components/shared/utils/reveal-detail-field'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import { createDebouncedDetailFetcher } from '@/components/shared/utils/debounced-detail-fetcher.svelte'
  import {
    columnID,
    loadColumns,
  } from '@/components/traces/Waterfall/waterfall-columns'

  let {
    importFailure = null,
    onViewIssue,
  }: {
    importFailure?: ImportFailure | null
    onViewIssue?: (event: MouseEvent) => void
  } = $props()

  let timeContext = getTimeContext()
  const routeContext = getRouteContext()

  let baselineStats = $state<TraceStats | null>(null)
  let polledStats = $state<TraceStats | null>(null)
  let actionError = $state<string | null>(null)
  let query = $state('')
  let submittedText = $state('')
  let fields = $state<FieldDefinition[]>([])
  let submittedRequest: ParsedSearchRequest | null = null
  const page = createSignalListPage<TraceSummary, TraceSummarySortColumn>({
    signal: 'traces',
    preserveMissingSelection: true,
    getItemID: trace => trace.traceID,
    initialSort: { column: 'startTime', direction: 'desc' },
    compare: compareTraceSummaries,
    fetchList: async () => {
      const { startTime, endTime } = selectionToQueryRangeNs(
        timeContext.selection,
        Date.now()
      )
      const results = await telemetryAPI.searchTraceSummaries(
        startTime,
        endTime,
        submittedRequest?.predicate ?? undefined,
        submittedRequest?.limit ?? undefined,
        untrack(() => ({
          field: page.sortColumn,
          direction: page.sortDirection,
        }))
      )
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
  let services = $derived([
    ...new Set(
      page.items.flatMap(trace =>
        trace.rootSpan?.serviceName ? [trace.rootSpan.serviceName] : []
      )
    ),
  ])

  // `/traces/<traceID>?span=<spanID>&event=<index>` — span/event in query string.
  let selectedSpanID = $derived(routeContext.route.query[SPAN_PARAM] ?? null)
  let selectedEventRaw = $derived(routeContext.route.query[EVENT_PARAM])
  let selectedEventIndex = $derived(parseEventIndex(selectedEventRaw))
  let selectedLogRef = $derived(routeContext.route.query[LOG_PARAM] ?? null)
  let traceData = $state<TraceWaterfallData | null>(null)
  let traceLogs = $state<TraceLogSummary[]>([])
  let detailLoading = $state(false)
  let detailError = $state('')
  let activeQueryTree = $state<QueryNode | undefined>(undefined)

  let hasTraceRows = $derived(page.items.length > 0)
  let displayError = $derived(page.error ?? actionError)

  let currentTraceData = $derived(page.selectedID ? traceData : null)
  // Requested and displayed identities stay separate during a handoff.
  let retainedTreeSpanID = $state<string | null>(null)
  let displayingRequestedTrace = $derived(
    currentTraceData?.traceID === page.selectedID
  )
  $effect.pre(() => {
    if (displayingRequestedTrace) {
      retainedTreeSpanID = selectedSpanID
    }
  })
  let displayedSpanID = $derived(
    displayingRequestedTrace ? selectedSpanID : retainedTreeSpanID
  )
  let selectedNode = $derived(
    currentTraceData?.spans.find(n => n.spanData.spanID === displayedSpanID) ??
      currentTraceData?.spans[0] ??
      undefined
  )
  let selectedSpan = $derived(selectedNode?.spanData)
  const spanCache = new Map<string, SpanData>()
  let detailEpoch = $state(0)
  const spanDetails = createDebouncedDetailFetcher<
    { traceID: string; spanID: string; epoch: number },
    SpanData
  >({
    keysEqual: (a, b) =>
      a.traceID === b.traceID && a.spanID === b.spanID && a.epoch === b.epoch,
    fetch: async (key, signal) => {
      const id = `${key.traceID}/${key.spanID}`
      const cached = spanCache.get(id)
      if (cached) {
        spanCache.delete(id)
        spanCache.set(id, cached)
        return cached
      }
      const result = await telemetryAPI.getSpan(key.traceID, key.spanID, signal)
      if (!result) throw new Error('The selected span was not found.')
      if (result.traceID !== key.traceID || result.spanID !== key.spanID)
        throw new Error(
          'The span Details response did not match the requested identity.'
        )
      if (!signal.aborted && key.epoch === detailEpoch) {
        spanCache.set(id, result)
        if (spanCache.size > 16)
          spanCache.delete(spanCache.keys().next().value!)
      }
      return result
    },
  })
  let detailIdentity = $derived(
    page.selectedID &&
      (selectedSpanID ||
        (displayingRequestedTrace ? selectedSpan?.spanID : null))
      ? {
          traceID: page.selectedID!,
          spanID: (selectedSpanID || selectedSpan?.spanID)!,
          epoch: detailEpoch,
        }
      : null
  )
  let fullSpan = $derived(
    spanDetails.data?.traceID === detailIdentity?.traceID &&
      spanDetails.data?.spanID === detailIdentity?.spanID
      ? (spanDetails.data ?? undefined)
      : undefined
  )

  let selectedColumns = $state<WaterfallColumn[]>(loadColumns())
  let previewValues = $state<TraceColumnPreview[]>([])
  let previewError = $state('')
  let previewTraceID = $state('')
  let loadedPreviewKey = $state('')
  const previewColumns = $derived(
    selectedColumns
      .filter(column => !column.field)
      .map(({ source, key }) => ({ source, key }))
  )
  $effect(() => {
    const trace = currentTraceData
    const columns = previewColumns
    if (!trace || trace.traceID !== page.selectedID) return
    const key = JSON.stringify([trace.traceID, columns])
    if (key === loadedPreviewKey) return
    const controller = new AbortController()
    previewError = ''
    void telemetryAPI
      .getTraceColumnPreviews(trace.traceID, columns, controller.signal)
      .then(
        values => {
          if (controller.signal.aborted) return
          previewTraceID = trace.traceID
          previewValues = values
          loadedPreviewKey = key
        },
        err => {
          if (!controller.signal.aborted && !isAbortError(err))
            previewError = 'Could not load waterfall column previews.'
        }
      )
    return () => controller.abort()
  })
  let waterfallSpans = $derived.by(() => {
    const bySpan = new Map<string, TraceColumnPreview[]>()
    if (previewTraceID === currentTraceData?.traceID)
      for (const cell of previewValues) {
        const cells = bySpan.get(cell.spanID) ?? []
        cells.push(cell)
        bySpan.set(cell.spanID, cells)
      }
    return (
      currentTraceData?.spans.map(node =>
        'columnValues' in node.spanData
          ? {
              ...node,
              spanData: {
                ...node.spanData,
                columnValues: bySpan.get(node.spanData.spanID) ?? [],
              },
            }
          : node
      ) ?? []
    )
  })
  let selectedSpanLogs = $derived(
    fullSpan && displayingRequestedTrace
      ? traceLogs.filter(log => log.spanID === fullSpan.spanID)
      : []
  )

  let resolvedEventIndex = $derived.by((): number | null => {
    const span =
      fullSpan ?? (displayingRequestedTrace ? selectedSpan : undefined)
    const index = selectedEventIndex
    if (index === null || !span) return null
    if (index >= span.events.length) return null
    return index
  })
  let resolvedLogRef = $derived.by((): string | null => {
    if (!displayingRequestedTrace) return null
    const logRef = selectedLogRef
    const span = selectedSpan
    if (!logRef || !span) return null
    return traceLogs.some(
      log => log.logRef === logRef && log.spanID === span.spanID
    )
      ? logRef
      : null
  })

  $effect(() => {
    const raw = selectedEventRaw
    if (detailLoading || !displayingRequestedTrace || currentTraceData === null)
      return

    if (resolvedEventIndex !== null) {
      if (selectedLogRef !== null) setEventInQuery(resolvedEventIndex)
      return
    }

    if (resolvedLogRef !== null) {
      if (raw !== undefined) setLogInQuery(resolvedLogRef)
      return
    }

    if (raw !== undefined) setEventInQuery(null)
    else if (selectedLogRef !== null) setLogInQuery(null)
  })

  $effect(() => {
    const summary = page.selectedSummary
    if (!summary) {
      // Don't tear down the detail view while the list is still loading -- a
      // shared link's trace id may simply not be in the list yet.
      if (!page.mounted || page.loading) return
      if (page.selectedID) {
        fetchTraceDetail(page.selectedID, activeQueryTree)
        return
      }
      clearTraceDetail()
      setSpanInQuery(null)
      return
    }
    fetchTraceDetail(summary.traceID, activeQueryTree)
  })

  // Activate the new snapshot before consulting its within-activation cache.
  $effect(() => spanDetails.select(detailIdentity))

  function selectTrace(traceID: string) {
    page.selectItem(traceID)
  }

  function handleSelectSpan(spanID: string) {
    if (!displayingRequestedTrace) return
    fieldRequest = undefined
    detailPanel.reveal()
    setSpanInQuery(spanID, 'push')
  }

  function handleSelectTimelineRecord(spanID: string, record: TimelineRecord) {
    if (!displayingRequestedTrace) return
    fieldRequest = undefined
    detailPanel.reveal()
    if (record.kind === 'event') {
      selectSpanEvent(spanID, record.eventIndex, 'push')
    } else if (record.kind === 'log') {
      selectSpanLog(spanID, record.log.logRef, 'push')
    }
  }

  let fieldRequest = $state<{
    traceID: string | null
    spanID: string
    target: DetailFieldTarget
  }>()

  function handleInspectColumn(spanID: string, column: WaterfallColumn) {
    handleSelectSpan(spanID)
    fieldRequest = {
      traceID: page.selectedID,
      spanID,
      target: {
        source: column.source,
        key: column.key,
        kind: column.field ? 'field' : 'attribute',
      },
    }
  }

  $effect(() => {
    if (
      fieldRequest &&
      (fieldRequest.spanID !== selectedSpanID ||
        fieldRequest.traceID !== page.selectedID)
    )
      fieldRequest = undefined
  })

  // Publish the complete Details presentation together. Pending requests retain
  // its own span, activity, links and action targets, never a new selection's
  // metadata combined with the previous record's values.
  let displayedDetail = $state.raw<{
    span: SpanData
    logs: TraceLogSummary[]
    rootSpanID?: string
    fieldTarget?: DetailFieldTarget
    salvaged: boolean
    cyclePoint: boolean
    selectedEventIndex: number | null
    selectedLogRef: string | null
  }>()
  $effect.pre(() => {
    if (!page.selectedID) {
      displayedDetail = undefined
      return
    }
    if (!fullSpan) return
    displayedDetail = {
      span: fullSpan,
      logs: selectedSpanLogs,
      rootSpanID: displayingRequestedTrace
        ? currentTraceData?.spans.find(node => !node.spanData.parentSpanID)
            ?.spanData.spanID
        : undefined,
      fieldTarget:
        fieldRequest?.traceID === fullSpan.traceID &&
        fieldRequest.spanID === fullSpan.spanID
          ? fieldRequest.target
          : undefined,
      salvaged: displayingRequestedTrace && (selectedNode?.salvaged ?? false),
      cyclePoint:
        displayingRequestedTrace && (selectedNode?.cyclePoint ?? false),
      selectedEventIndex: resolvedEventIndex,
      selectedLogRef: resolvedLogRef,
    }
  })

  function handleSearchResults(event: SearchResultEvent) {
    page.handleSearchResults(event)
    if (event.signal === 'traces') {
      // SAFETY: The traces search producer attaches a QueryNode queryTree to this event variant.
      activeQueryTree = event.queryTree as QueryNode | undefined
    }
  }

  // Abort superseded detail queries so they release the store read lock.
  let detailFetch: AbortController | null = null
  let requestedTraceID: string | null = null

  onDestroy(() => detailFetch?.abort())

  function clearTraceDetail() {
    detailFetch?.abort()
    detailFetch = null
    requestedTraceID = null
    spanCache.clear()
    spanDetails.select(null)
    displayedDetail = undefined
    traceData = null
    traceLogs = []
    previewValues = []
    previewTraceID = ''
    loadedPreviewKey = ''
    previewError = ''
    detailLoading = false
    detailError = ''
  }

  async function fetchTraceDetail(traceID: string, queryTree?: QueryNode) {
    const retainedSpanID = untrack(() => getSpanFromQuery())
    const requestedColumns = untrack(() => previewColumns)
    detailFetch?.abort()
    const fetchCtl = new AbortController()
    detailFetch = fetchCtl
    if (requestedTraceID !== null) {
      spanCache.clear()
      detailEpoch = untrack(() => detailEpoch) + 1
    }
    requestedTraceID = traceID

    try {
      detailLoading = true
      detailError = ''
      previewError = ''
      const [result, logs] = await Promise.all([
        telemetryAPI.getTraceView(
          traceID,
          queryTree,
          fetchCtl.signal,
          requestedColumns
        ),
        telemetryAPI.getTraceLogSummaries(traceID, fetchCtl.signal),
      ])
      if (detailFetch !== fetchCtl || page.selectedID !== traceID) return
      traceData = result
      previewTraceID = traceID
      previewValues = result.spans.flatMap(node =>
        'columnValues' in node.spanData ? node.spanData.columnValues : []
      )
      loadedPreviewKey = JSON.stringify([traceID, requestedColumns])
      traceLogs = logs
      const spanIDs = result.spans.map(n => n.spanData.spanID)
      const urlSpan = getSpanFromQuery()
      let desired: string | null
      if (
        retainedSpanID &&
        retainedSpanID === urlSpan &&
        spanIDs.includes(retainedSpanID)
      ) {
        desired = retainedSpanID
      } else if (queryTree) {
        const firstMatch = result.spans.find(n => n.matched)
        desired = firstMatch?.spanData.spanID ?? spanIDs[0] ?? null
      } else if (urlSpan && spanIDs.includes(urlSpan)) {
        desired = urlSpan
      } else {
        desired = spanIDs[0] ?? null
      }
      if (desired !== urlSpan) setSpanInQuery(desired)
    } catch (err) {
      if (detailFetch !== fetchCtl) return
      fetchCtl.abort()
      // A superseded fetch is not a failure: a newer one is already in flight
      // and owns the view state.
      if (isAbortError(err)) return
      console.error('Failed to fetch trace detail:', err)
      detailError = 'Could not load the selected trace.'
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
    if (!confirmDeleteAll('trace')) return
    actionError = null
    try {
      await telemetryAPI.clearTraces()
      clearTraceDetail()
      navigateToItem('traces', null, 'replace')
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
      if (page.selectedID === traceID) {
        clearTraceDetail()
        navigateToItem('traces', null, 'replace')
      }
      await page.runListFetch()
    } catch (err) {
      actionError =
        err instanceof Error ? err.message : 'Failed to delete trace'
      console.error('Error deleting trace:', err)
    }
  }

  function deleteSelectedTrace() {
    if (currentTraceData) handleDeleteTrace(currentTraceData.traceID)
  }
</script>

<PageLayout
  mainSurface="base"
  items={page.sortedItems}
  selectedID={page.selectedID}
  drawerID="signal-drawer"
  drawerLabel="Traces"
  onRefresh={page.handleRefresh}
  refreshPulse={page.refreshPulse}
  refreshAsideTip={page.refreshAsideTip}
  loading={page.loading}
  loadingRequestKey={`${page.requestKey}:${page.selectedID}:${selectedSpanID}:${detailEpoch}`}
  loadingMessage={detailLoading
    ? 'Loading selected trace…'
    : spanDetails.loading
      ? 'Loading selected span…'
      : page.loading
        ? 'Loading traces…'
        : undefined}
  itemKey={t => t.traceID}
>
  {#snippet drawerSearch()}
    <DrawerSearchPanel
      segment="search"
      signal="traces"
      sortValue={page.sortColumn}
      sortDirection={page.sortDirection}
      onSearchResults={handleSearchResults}
      onSearchReady={api => (page.searchEditorApi = api)}
      onQueryChange={(text, availableFields) => {
        query = text
        fields = availableFields
      }}
      onQuerySubmit={text => {
        submittedText = text
        submittedRequest = parseSearchRequest(text, fields, 'traces')
      }}
    />
  {/snippet}

  {#snippet drawerControls()}
    <ListHeaderControls label="Trace list controls">
      {#snippet sort()}
        <DrawerSortControl
          options={[
            { value: 'startTime', label: 'Start Time' },
            { value: 'duration', label: 'Duration' },
            { value: 'rootSpanName', label: 'Root Span Name' },
            { value: 'serviceName', label: 'Service Name' },
            { value: 'spanCount', label: 'Span Count' },
            { value: 'errorCount', label: 'Error Count' },
          ]}
          value={page.sortColumn}
          direction={page.sortDirection}
          onchange={page.handleSortChange}
        />
      {/snippet}
      {#snippet filters()}<TelemetryListFilters
          signal="traces"
          {query}
          appliedQuery={submittedText}
          {fields}
          {services}
          editor={page.searchEditorApi}
        />{/snippet}
    </ListHeaderControls>
  {/snippet}

  {#snippet itemSnippet(trace, selected)}
    <TraceCard {trace} {selected} onclick={selectTrace} />
  {/snippet}

  {#snippet drawerFooter()}
    <SignalDrawerFooter label="trace" onDeleteAll={handleDeleteAllTraces} />
  {/snippet}

  {#snippet main()}
    {#if detailError}
      <p class="traces-page__placeholder" role="alert">{detailError}</p>
    {/if}
    {#if previewError}<p class="traces-page__placeholder" role="alert">
        {previewError}
      </p>{/if}
    {#if displayError || importFailure}
      <div class="traces-page__placeholder alert alert-error">
        {#if displayError}
          <span>Error: {displayError}</span>
        {:else if importFailure}
          <span>Import issue in {importFailure.fileName}.</span>
          <a class="link" href="/#{INGESTION_ISSUES_ID}" onclick={onViewIssue}
            >View issue</a
          >
        {/if}
      </div>
    {:else if page.loading && !hasTraceRows && !currentTraceData}
      <div class="traces-page__placeholder traces-empty"></div>
    {:else if !page.loading && !hasTraceRows && !currentTraceData}
      <div class="traces-page__placeholder traces-empty">
        <p class="text-subtle">No traces in this time range</p>
        <p class="mt-2 text-sm text-subtle">
          Send telemetry to the exporter or adjust the time range
        </p>
      </div>
    {:else if currentTraceData}
      {#if currentTraceData.unplacedSpanCount > 0}
        <div class="traces-page__unplaced alert alert-warning" role="alert">
          <span>
            {currentTraceData.unplacedSpanCount}
            {currentTraceData.unplacedSpanCount === 1 ? 'span is' : 'spans are'} missing
            from this trace. Their parent links form a loop, so they have no place
            in the tree — usually an instrumentation bug in the service that emitted
            them.
          </span>
        </div>
      {/if}
      <WaterfallView
        traceID={currentTraceData?.traceID ?? ''}
        spans={waterfallSpans}
        columnCatalogue={currentTraceData.availableColumns}
        onColumnsChange={columns => {
          if (
            columns.map(columnID).join() !==
            selectedColumns.map(columnID).join()
          )
            selectedColumns = columns
        }}
        logs={traceLogs}
        selectedSpanID={displayedSpanID}
        searchActive={activeQueryTree !== undefined}
        onSelectSpan={handleSelectSpan}
        onInspectColumn={handleInspectColumn}
        onSelectTimelineRecord={handleSelectTimelineRecord}
      />
    {:else if detailLoading || detailError}
      <div class="traces-page__placeholder traces-empty"></div>
    {:else}
      <div class="traces-page__placeholder traces-empty">
        <p class="text-subtle">Select a trace to view details</p>
      </div>
    {/if}
  {/snippet}

  {#snippet detail()}
    <DetailView
      span={displayedDetail?.span}
      loading={(detailLoading && !fullSpan) ||
        spanDetails.loading ||
        (!!detailIdentity && !fullSpan && !spanDetails.error)}
      error={spanDetails.error ?? ''}
      rootSpanID={displayedDetail?.rootSpanID}
      fieldTarget={displayedDetail?.fieldTarget}
      salvaged={displayedDetail?.salvaged}
      cyclePoint={displayedDetail?.cyclePoint}
      selectedEventIndex={displayedDetail?.selectedEventIndex}
      selectedLogRef={displayedDetail?.selectedLogRef}
      logs={displayedDetail?.logs ?? []}
    />
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
      onDelete={currentTraceData ? deleteSelectedTrace : undefined}
    />
  {/snippet}
</PageLayout>

<style lang="postcss">
  @reference "../app.css";

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
