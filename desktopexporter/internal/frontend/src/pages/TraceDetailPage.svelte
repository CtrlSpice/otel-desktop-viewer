<script lang="ts">
  import { router } from 'tinro5';
  import { telemetryAPI } from '@/services/telemetry-service';
  import type { TraceData, SearchResultEvent } from '@/types/api-types';
  import { stashNavState } from '@/utils/nav-state';
  import SignalHeader from '@/components/SignalHeader/SignalHeader.svelte';
  import DetailView from '@/components/TraceDetails/DetailView.svelte';
  import WaterfallView from '@/components/TraceDetails/WaterfallView.svelte';
  import ResizablePanels from '@/components/ResizablePanels.svelte';

  // --- state: route + API ---
  let traceID = $state<string>('');
  let traceData = $state<TraceData | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let loadSeq = 0;

  // --- state: span selection ---
  let selectedSpanID = $state<string | null>(null);

  let selectedSpan = $derived(
    traceData?.spans.find(n => n.spanData.spanID === selectedSpanID)
      ?.spanData ??
      traceData?.spans[0]?.spanData ??
      undefined
  );

  // --- effects ---

  $effect(() => {
    stashNavState('appPage', true);
  });

  $effect(() => {
    const unsubscribe = router.subscribe(route => {
      const match = route.path.match(/^\/trace\/(.+)$/);
      if (match && match[1]) {
        traceID = match[1];
      } else {
        error = 'No trace ID provided';
        loading = false;
      }
    });
    return unsubscribe;
  });

  $effect(() => {
    if (traceID) {
      fetchTrace();
    }
  });

  // --- handlers ---

  async function fetchTrace() {
    if (!traceID) return;
    const seq = ++loadSeq;
    loading = true;
    error = null;
    try {
      const result = await telemetryAPI.getTraceByID(traceID);
      if (seq !== loadSeq) return;
      traceData = result;
      selectedSpanID = result.spans[0]?.spanData.spanID ?? null;
    } catch (err) {
      if (seq !== loadSeq) return;
      error = err instanceof Error ? err.message : 'Failed to fetch trace';
      console.error('Error fetching trace:', err);
    } finally {
      if (seq === loadSeq) {
        loading = false;
      }
    }
  }

  const handleBack = () => {
    if (history.length > 1) {
      history.back();
    } else {
      router.goto('/traces');
    }
  };

  const handleSelectSpan = (spanID: string) => {
    selectedSpanID = spanID;
  };

  const handleSearchResults = (event: SearchResultEvent) => {
    if (event.signal === 'traces' && event.view === 'detail') {
      const result = event.results as unknown as TraceData;
      traceData = result;
      selectedSpanID = result.spans[0]?.spanData.spanID ?? null;
    }
  };
</script>

<div class="flex min-w-0 w-full flex-col overflow-y-auto py-6">
  <SignalHeader
    signal="traces"
    view="detail"
    {traceID}
    onBack={handleBack}
    onRefresh={fetchTrace}
    onSearchResults={handleSearchResults}
  />

  {#if error}
    <div class="alert alert-error mb-4">
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
    <div class="h-[calc(100vh-10rem)]">
      <ResizablePanels
        defaultLeftWidth={0.6}
        minLeftWidth={0.3}
        minRightWidth={0.3}
        storageKey="trace-detail-panels"
      >
        {#snippet leftPanel()}
          <div
            class="h-full rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm overflow-hidden transition-opacity duration-200 {loading
              ? 'opacity-70'
              : 'opacity-100'}"
          >
            <WaterfallView
              spans={data.spans}
              {selectedSpanID}
              onSelectSpan={handleSelectSpan}
            />
          </div>
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
