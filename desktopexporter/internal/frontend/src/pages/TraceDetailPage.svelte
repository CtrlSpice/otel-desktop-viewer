<script lang="ts">
  import { router } from 'tinro5'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type { TraceData, SearchResultEvent } from '@/types/api-types'
  import { stashNavState } from '@/utils/nav-state'
  import SignalToolbar from '@/components/SignalToolbar/SignalToolbar.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import { traceDetailStats } from '@/utils/trace-detail-stats'
  import DetailView from '@/components/TraceDetails/DetailView/DetailView.svelte'
  import WaterfallView from '@/components/TraceDetails/Waterfall/WaterfallView.svelte'
  import ResizablePanels from '@/components/ResizablePanels.svelte'

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

  // Column filter (FieldFilter in toolbar + DetailView columnFilter): deferred for this
  // release. FieldFilter / helpers stay in repo; restore snippet + state, trailingFilters, and
  // columnFilter={columnFilterSelection} on DetailView (empty selection = show all columns).

  // --- effects ---

  $effect(() => {
    stashNavState('appPage', true)
  })

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
    if (history.length > 1) {
      history.back()
    } else {
      router.goto('/traces')
    }
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
</script>

<div
  class="flex min-h-0 min-w-0 w-full flex-1 flex-col gap-[var(--layout-gap)] pb-6 pt-0"
>
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
