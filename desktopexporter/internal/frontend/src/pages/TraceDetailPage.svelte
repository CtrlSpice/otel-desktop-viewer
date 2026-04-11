<script lang="ts">
  import { router } from 'tinro5'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type { TraceData, SearchResultEvent } from '@/types/api-types'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import {
    getTraceListNavIds,
    setTraceListNavIds,
  } from '@/stores/trace-list-nav.svelte'
  import { sortTraceSummaries } from '@/utils/trace-summary-sort'
  import { loadTraceListTableState } from '@/utils/trace-list-table-state'
  import type { SearchEditorAPI } from '@/components/SignalToolbar/search/search-editor-api'
  import SignalToolbar from '@/components/SignalToolbar/SignalToolbar.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import { traceDetailStats } from '@/utils/trace-detail-stats'
  import DetailView from '@/components/TraceDetails/DetailView/DetailView.svelte'
  import WaterfallView from '@/components/TraceDetails/Waterfall/WaterfallView.svelte'
  import ResizablePanels from '@/components/ResizablePanels.svelte'
  import {
    ArrowLeftDoubleIcon,
    ArrowLeftIcon,
    ArrowRightDoubleIcon,
    ArrowRightIcon,
  } from '@/icons'

  // --- state: route + API ---
  let traceID = $state<string>('')
  let traceData = $state<TraceData | null>(null)
  let loading = $state(true)
  let error = $state<string | null>(null)
  let loadSeq = 0
  let searchError = $state<string | null>(null)

  // --- state: span selection ---
  let selectedSpanID = $state<string | null>(null)

  // --- state: polling / refresh indicator ---
  let searchEditorApi = $state<SearchEditorAPI | null>(null)
  let baselineSpanCount = $state(0)
  let polledSpanCount = $state(0)
  const POLL_INTERVAL_MS = 3000

  let selectedSpan = $derived(
    traceData?.spans.find(n => n.spanData.spanID === selectedSpanID)
      ?.spanData ??
      traceData?.spans[0]?.spanData ??
      undefined
  )

  let traceStats = $derived(traceData ? traceDetailStats(traceData) : null)

  let timeContext = getTimeContext()

  let navIds = $derived(getTraceListNavIds())
  let traceNavIndex = $derived(navIds.indexOf(traceID))
  let traceNavTotal = $derived(navIds.length)
  let traceNavPositionLabel = $derived(
    traceNavIndex >= 0 ? String(traceNavIndex + 1) : '—'
  )
  let canGoPrev = $derived(traceNavIndex > 0)
  let canGoNext = $derived(
    traceNavIndex >= 0 && traceNavIndex < traceNavTotal - 1
  )

  let refreshIndicatorText = $derived.by(() => {
    const delta = polledSpanCount - baselineSpanCount
    if (delta <= 0) return ''
    return `+${delta} span${delta !== 1 ? 's' : ''}`
  })

  // --- effects ---

  $effect(() => {
    const unsubscribe = router.subscribe(route => {
      const match = route.path.match(/^\/trace\/(.+)$/)
      if (match && match[1]) {
        traceID = match[1]
      } else {
        error = 'No trace ID provided'
        loading = false
      }
    })
    return unsubscribe
  })

  // Trace data fetching is driven by the SearchEditor's re-fire $effect,
  // which calls onSubmit() on every traceID change. This ensures a single
  // fetch per navigation — with the active query when present, or a clean
  // fetch when the editor is empty. Results arrive via handleSearchResults.
  $effect(() => {
    if (!traceID) return
    loading = true
    error = null
  })

  async function backfillTraceListIfEmpty() {
    if (getTraceListNavIds().length > 0) return
    try {
      let { start, end } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      let { sortColumn, sortDirection } = loadTraceListTableState()
      let summaries = await telemetryAPI.searchTraces(start, end)
      let sorted = sortTraceSummaries(summaries, sortColumn, sortDirection)
      setTraceListNavIds(sorted.map(s => s.traceID))
    } catch (e) {
      console.warn('Trace list backfill failed:', e)
    }
  }

  $effect(() => {
    if (!traceID) return
    if (getTraceListNavIds().length > 0) return
    void backfillTraceListIfEmpty()
  })

  $effect(() => {
    if (!traceID) return
    const tid = traceID
    const id = setInterval(async () => {
      try {
        polledSpanCount = await telemetryAPI.getTraceSpanCount(tid)
      } catch { /* polling failures are silent */ }
    }, POLL_INTERVAL_MS)
    return () => clearInterval(id)
  })

  // --- handlers ---

  async function fetchTrace() {
    if (!traceID) return
    const seq = ++loadSeq
    loading = true
    error = null
    try {
      const result = await telemetryAPI.searchSpans(traceID)
      if (seq !== loadSeq) return
      traceData = result
      selectedSpanID = result.spans[0]?.spanData.spanID ?? null
      baselineSpanCount = result.spans.length
      polledSpanCount = result.spans.length
    } catch (err) {
      if (seq !== loadSeq) return
      const msg = err instanceof Error ? err.message : ''
      if (msg.includes('Trace not found')) {
        traceData = null
      } else {
        error = msg || 'Failed to fetch trace'
        console.error('Error fetching trace:', err)
      }
    } finally {
      if (seq === loadSeq) {
        loading = false
      }
    }
  }

  function handleRefresh() {
    searchEditorApi?.clear()
    fetchTrace()
  }

  const handleBack = () => {
    router.goto('/traces')
  }

  const handleSelectSpan = (spanID: string) => {
    selectedSpanID = spanID
  }

  const handleSearchResults = (event: SearchResultEvent) => {
    if (event.signal === 'traces' && event.view === 'detail') {
      traceData = event.results
      loading = false
      error = null
      baselineSpanCount = event.results.spans.length
      polledSpanCount = event.results.spans.length
      const stillExists = event.results.spans.some(
        n => n.spanData.spanID === selectedSpanID
      )
      if (!stillExists) {
        const firstMatch = event.results.spans.find(n => n.matched)
        selectedSpanID = firstMatch?.spanData.spanID ?? event.results.spans[0]?.spanData.spanID ?? null
      }
    }
  }

  function goTraceByIndex(i: number) {
    let id = navIds[i]
    if (id) router.goto(`/trace/${id}`)
  }

  function goTraceNavFirst() {
    if (canGoPrev) goTraceByIndex(0)
  }
  function goTraceNavPrev() {
    if (canGoPrev) goTraceByIndex(traceNavIndex - 1)
  }
  function goTraceNavNext() {
    if (canGoNext) goTraceByIndex(traceNavIndex + 1)
  }
  function goTraceNavLast() {
    if (canGoNext) goTraceByIndex(traceNavTotal - 1)
  }
</script>

{#snippet traceNavFooter()}
  {#if navIds.length > 0}
    <div class="pagination-controls">
      <div class="pagination-rows-selector"></div>
      <div class="pagination-controls__center">
        <div
          class="flex min-w-0 flex-nowrap items-center justify-center gap-1.5"
        >
          <button
            type="button"
            class="btn btn-ghost btn-sm btn-circle"
            disabled={!canGoPrev}
            onclick={goTraceNavFirst}
            aria-label="First trace in list"
          >
            <ArrowLeftDoubleIcon class="h-4 w-4" aria-hidden="true" />
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-sm btn-circle"
            disabled={!canGoPrev}
            onclick={goTraceNavPrev}
            aria-label="Previous trace in list"
          >
            <ArrowLeftIcon class="h-4 w-4" aria-hidden="true" />
          </button>
          <div
            class="flex min-h-8 min-w-[10rem] items-center justify-center rounded-lg bg-base-200/50 px-3 text-sm tabular-nums text-base-content/70"
          >
            {traceNavPositionLabel} of {traceNavTotal} traces
          </div>
          <button
            type="button"
            class="btn btn-ghost btn-sm btn-circle"
            disabled={!canGoNext}
            onclick={goTraceNavNext}
            aria-label="Next trace in list"
          >
            <ArrowRightIcon class="h-4 w-4" aria-hidden="true" />
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-sm btn-circle"
            disabled={!canGoNext}
            onclick={goTraceNavLast}
            aria-label="Last trace in list"
          >
            <ArrowRightDoubleIcon class="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </div>
      <div class="pagination-controls__actions"></div>
    </div>
  {/if}
{/snippet}

<div
  class="flex min-h-0 min-w-0 w-full flex-1 flex-col gap-[var(--layout-gap)] pt-0"
>
  <div class="page-toolbar-block">
    <SignalToolbar
      signal="traces"
      view="detail"
      {traceID}
      {traceStats}
      onBack={handleBack}
      onRefresh={handleRefresh}
      trailingFilters={[]}
      {searchError}
      {refreshIndicatorText}
    >
      <SearchEditor
        signal="traces"
        view="detail"
        {traceID}
        inToolbar
        onSearchResults={handleSearchResults}
        onSearchError={(err) => (searchError = err)}
        onReady={(api) => (searchEditorApi = api)}
      />
    </SignalToolbar>
  </div>

  {#if error}
    <div class="alert alert-error">
      <span>Error: {error}</span>
    </div>
  {/if}

  {#if loading && !traceData}
    <div
      class="rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center text-base-content/60 shadow-surface-sm backdrop-blur-sm"
    >
      Loading trace…
    </div>
  {:else if traceData}
    {@const data = traceData}
    <div class="min-h-0 flex-1">
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
            {loading}
            footer={traceNavFooter}
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
  {:else if !loading}
    <div
      class="rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center shadow-surface-sm backdrop-blur-sm"
    >
      <p class="text-base-content/60">Trace not found</p>
    </div>
  {/if}
</div>
