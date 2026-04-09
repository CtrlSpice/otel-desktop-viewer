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

  // --- state: span selection ---
  let selectedSpanID = $state<string | null>(null)

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

  // Column filter (FieldFilter in toolbar + DetailView columnFilter): deferred for this
  // release. FieldFilter / helpers stay in repo; restore snippet + state, trailingFilters, and
  // columnFilter={columnFilterSelection} on DetailView (empty selection = show all columns).

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

  $effect(() => {
    if (traceID) {
      fetchTrace()
    }
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

  // --- handlers ---

  async function fetchTrace() {
    if (!traceID) return
    const seq = ++loadSeq
    loading = true
    error = null
    try {
      const result = await telemetryAPI.getTraceByID(traceID)
      if (seq !== loadSeq) return
      traceData = result
      selectedSpanID = result.spans[0]?.spanData.spanID ?? null
    } catch (err) {
      if (seq !== loadSeq) return
      error = err instanceof Error ? err.message : 'Failed to fetch trace'
      console.error('Error fetching trace:', err)
    } finally {
      if (seq === loadSeq) {
        loading = false
      }
    }
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
      selectedSpanID = event.results.spans[0]?.spanData.spanID ?? null
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
            {traceNavPositionLabel} of {traceNavTotal}
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
      onRefresh={fetchTrace}
      trailingFilters={[]}
    />
    <SearchEditor
      signal="traces"
      view="detail"
      {traceID}
      inToolbar
      onSearchResults={handleSearchResults}
    />
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
