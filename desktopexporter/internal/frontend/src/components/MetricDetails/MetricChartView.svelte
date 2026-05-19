<script lang="ts">
  /*
   * MetricChartView is the "main" pane on the metrics page. It owns
   * the chart surface (one of three: histogram tabs, time-series chart,
   * or an unspecified-temporality callout), the per-chart legend
   * column, and the placeholder for metric types that don't have a
   * native chart yet.
   *
   * It does NOT own selection, filters, fetches, or the bucket-series
   * lifecycle: those live in MetricViewContext. The chart view is a
   * pure renderer of the context's derivations and a thin invoker of
   * its methods (setActiveHistogramTab, onHeatmapSelect). The same
   * context is read by MetricDetailView for the Fields/Datapoints
   * pane, so both panes stay in lockstep without prop chains.
   */
  import {
    getMetricViewContext,
    type BucketSeriesError,
  } from '@/contexts/metric-view-context.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { selectionToQueryRangeMs } from '@/contexts/time-context.svelte'
  import { formatTimestamp } from '@/utils/time'
  import MetricTimeSeriesChart from '@/components/MetricCharts/MetricTimeSeriesChart.svelte'
  import HistogramChart from '@/components/MetricCharts/HistogramChart.svelte'
  import HistogramHeatmap from '@/components/MetricCharts/HistogramHeatmap.svelte'
  import UnspecifiedTemporalityCallout from '@/components/MetricDetails/UnspecifiedTemporalityCallout.svelte'
  import {
    CameraIcon,
    ChartHistogramIcon,
    TemperatureIcon,
  } from '@/icons'

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  // Time window for the chart (used by histogram heatmap + aggregated
  // chart bounds). Re-derived here rather than read from the context
  // so the context doesn't need to expose a window field; this is
  // cheap.
  let queryRange = $derived(
    selectionToQueryRangeMs(timeContext.selection, Date.now())
  )
</script>

{#snippet bucketSeriesErrorMessage(err: BucketSeriesError)}
  <div class="metric-chart-view__placeholder text-error/70">
    {#if err.kind === 'unspecified'}
      Aggregation temporality is Unspecified — backend can't safely combine
      these datapoints.
    {:else if err.kind === 'boundsMismatch'}
      Histogram bounds disagree across datapoints in this window — backend can't
      merge.
    {:else}
      {err.message}
    {/if}
  </div>
{/snippet}

{#snippet histogramChartSlot()}
  <!-- Three tabs share this slot. The tab strip keeps the chart frame
       a constant pixel height regardless of which view is active so
       the bottom split panels don't reflow when the user switches
       tabs. Errors and loading states are unified across tabs because
       they all derive from the same fetches (bucket series + aggregated
       point). -->
  <div class="metric-chart-view">
    <div class="metric-chart-view__tabs" role="tablist" aria-label="Histogram views">
      <button
        type="button"
        role="tab"
        class="detail-tab {ctx.activeHistogramTab === 'heatmap'
          ? 'detail-tab--active'
          : 'detail-tab--inactive'}"
        aria-selected={ctx.activeHistogramTab === 'heatmap'}
        onclick={() => ctx.setActiveHistogramTab('heatmap')}
      >
        <TemperatureIcon class="w-4 h-4 shrink-0" />
        <span>Heatmap</span>
      </button>
      <button
        type="button"
        role="tab"
        class="detail-tab {ctx.activeHistogramTab === 'aggregated'
          ? 'detail-tab--active'
          : 'detail-tab--inactive'}"
        aria-selected={ctx.activeHistogramTab === 'aggregated'}
        onclick={() => ctx.setActiveHistogramTab('aggregated')}
      >
        <ChartHistogramIcon class="w-4 h-4 shrink-0" />
        <span>Aggregated</span>
      </button>
      <button
        type="button"
        role="tab"
        class="detail-tab {ctx.activeHistogramTab === 'snapshot'
          ? 'detail-tab--active'
          : 'detail-tab--inactive'}"
        aria-selected={ctx.activeHistogramTab === 'snapshot'}
        onclick={() => ctx.setActiveHistogramTab('snapshot')}
      >
        <CameraIcon class="w-4 h-4 shrink-0" />
        <span>Snapshot</span>
      </button>
    </div>
    <div class="metric-chart-view__row">
      <div class="metric-chart-view__body">
        {#if ctx.activeHistogramTab === 'heatmap'}
          {#if ctx.bucketSeriesError}
            {@render bucketSeriesErrorMessage(ctx.bucketSeriesError)}
          {:else if ctx.bucketSeriesLoading || ctx.visibleBucketSeries === null}
            <div class="metric-chart-view__placeholder">Loading heatmap…</div>
          {:else}
            <HistogramHeatmap
              points={ctx.visibleBucketSeries}
              windowStartMs={queryRange.start}
              windowEndMs={queryRange.end}
              height={250}
              onSelect={ctx.onHeatmapSelect}
              selectedTimestamp={ctx.heatmapSelectedTimestamp}
            />
          {/if}
        {:else if ctx.activeHistogramTab === 'aggregated'}
          {#if ctx.aggregatedError}
            {@render bucketSeriesErrorMessage(ctx.aggregatedError)}
          {:else if ctx.aggregatedLoading || !ctx.aggregatedDatapoint}
            <div class="metric-chart-view__placeholder">Loading aggregate…</div>
          {:else}
            <HistogramChart
              datapoint={ctx.aggregatedDatapoint}
              unit={ctx.metric!.unit}
              quantileSource="merged"
              metricID={ctx.metric!.id}
              windowStartMs={queryRange.start}
              windowEndMs={queryRange.end}
            />
          {/if}
        {:else if ctx.activeHistogramTab === 'snapshot'}
          {#if ctx.activeHistogramDp}
            <div class="metric-chart-view__subtitle">
              <span>datapoint at</span>
              <span class="tabular-nums">
                {formatTimestamp(
                  ctx.activeHistogramDp.timestamp,
                  timeContext.timezone,
                  'milliseconds'
                )}
              </span>
            </div>
            <HistogramChart
              datapoint={ctx.activeHistogramDp}
              unit={ctx.metric!.unit}
              quantileSource="datapoint"
            />
          {:else}
            <div class="metric-chart-view__placeholder">
              No datapoint selected
            </div>
          {/if}
        {/if}
      </div>
    </div>
  </div>
{/snippet}

{#snippet timeSeriesChartSlot()}
  <div class="metric-chart-view">
    <div class="metric-chart-view__row">
      <div class="metric-chart-view__body">
        <MetricTimeSeriesChart
          timeseries={ctx.gaugeSumChartTimeseries}
          visibleKeys={ctx.gaugeSumVisible}
          highlightedTimestamp={ctx.highlightedTimestamp}
        />
      </div>
    </div>
  </div>
{/snippet}

{#if !ctx.metric}
  <div class="metric-chart-view metric-chart-view--empty">
    <p class="text-base-content/40 text-sm">Select a metric to view details</p>
  </div>
{:else if ctx.isUnspecifiedTemporality}
  <!-- FunError takes the entire chart row. The detail pane (Fields /
       Datapoints) still renders independently because it consumes
       different data; this branch only blanks the chart. -->
  <div class="metric-chart-view">
    <UnspecifiedTemporalityCallout size="full" />
  </div>
{:else if ctx.isHistogramKind}
  {@render histogramChartSlot()}
{:else if ctx.metricType === 'Gauge' || ctx.metricType === 'Sum'}
  {@render timeSeriesChartSlot()}
{:else}
  <div class="metric-chart-view">
    <div class="metric-chart-view__placeholder">
      No chart available for this metric type
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  /* Outer chart container. shrink-0 in the parent flex column so the
     chart keeps its requested height; the detail pane below claims
     the remaining vertical space. */
  .metric-chart-view {
    @apply shrink-0 px-2 py-3;
  }

  .metric-chart-view--empty {
    @apply flex h-full items-center justify-center;
  }

  .metric-chart-view__row {
    @apply mt-2 flex items-stretch gap-3;
  }

  /* Chart body claims remaining width; min-w-0 lets it shrink past
     intrinsic content size when the legend column claims its share. */
  .metric-chart-view__body {
    @apply mt-2 min-w-0 flex-1;
    /* Override the mt-2 above when inside __row, which already
       supplies the top spacing. Doubling would push the chart down. */
    margin-top: 0;
  }

  .metric-chart-view__placeholder {
    @apply flex items-center justify-center text-base-content/40 text-sm;
    height: 250px;
  }

  /* Inline subtitle for the Snapshot tab. Sits right under the tab
     strip and tells the user WHICH datapoint they're looking at --
     critical context now that the chart only shows one snapshot at a
     time. */
  .metric-chart-view__subtitle {
    @apply mt-1 mb-1 flex items-baseline gap-2 px-2;
  }

  /* Histogram view tabs -- same pill treatment as Trace DetailView
     (detail-tab / detail-tab--active from app.css). Three controls
     share width evenly. */
  .metric-chart-view__tabs {
    @apply flex w-full flex-nowrap items-center gap-1 px-1;

    & > :global(button) {
      flex: 1 1 0%;
      justify-content: center;
      padding-top: 0.25rem;
      padding-bottom: 0.25rem;
      font-size: 0.75rem;
    }
  }
</style>
