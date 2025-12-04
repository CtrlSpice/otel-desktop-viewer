<script lang="ts">
  import { router } from 'tinro5';
  import { telemetryAPI } from '@/services/telemetry-service';
  import type { TraceData } from '@/types/api-types';
  import DetailView from '@/components/TraceDetails/DetailView.svelte';

  let traceID = $state<string>('');
  let traceData = $state<TraceData | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Extract traceID from path parameter
  $effect(() => {
    let unsubscribe = router.subscribe(route => {
      // Parse /trace/:traceID from route.path
      let match = route.path.match(/^\/trace\/(.+)$/);
      if (match && match[1]) {
        traceID = match[1];
      } else {
        error = 'No trace ID provided';
        loading = false;
      }
    });
    return unsubscribe;
  });

  // Fetch trace data when traceID changes
  $effect(() => {
    if (traceID) {
      fetchTrace();
    }
  });

  // TODO: Update when integrating search - will need to use searchTraceSpans endpoint
  // with time range and query filters from PageHeader
  async function fetchTrace() {
    if (!traceID) return;

    try {
      loading = true;
      error = null;

      traceData = await telemetryAPI.getTraceByID(traceID);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to fetch trace';
      console.error('Error fetching trace:', err);
    } finally {
      loading = false;
    }
  }

  function handleBack() {
    router.goto('/traces');
  }
</script>

<div class="flex flex-col w-full h-full">
  <!-- Back button -->
  <div class="mb-4">
    <button class="btn btn-sm btn-ghost" onclick={handleBack}>
      <svg class="w-4 h-4" viewBox="0 0 24 24">
        <path d="M15 19l-7-7 7-7" />
      </svg>
      <span>Back to Traces</span>
    </button>
  </div>

  {#if loading}
    <div class="flex items-center justify-center h-64">
      <span class="text-base-content/60">Loading trace...</span>
    </div>
  {:else if error}
    <div class="alert alert-error">
      <span>Error: {error}</span>
    </div>
  {:else if traceData}
    <!-- Waterfall + detail view -->
    <div class="flex gap-4 h-[calc(100vh-12rem)]">
      <div class="flex-1 border border-base-300 rounded-lg p-4">
        <h2 class="text-lg font-semibold mb-4">Waterfall View</h2>
        <p class="text-base-content/60">Trace ID: {traceData.traceID}</p>
        <p class="text-base-content/60">Spans: {traceData.spans.length}</p>
        <p class="text-base-content/60 mt-2">Waterfall view coming soon...</p>
      </div>
      <div class="overflow-hidden">
        {#if traceData.spans.length > 0}
          <DetailView span={traceData.spans[0].spanData} />
        {:else}
          <div class="p-4">
            <p class="text-base-content/60">No spans available</p>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  /* Component-specific styles */
</style>
