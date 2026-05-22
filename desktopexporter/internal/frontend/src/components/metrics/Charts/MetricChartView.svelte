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
   * context is read by MetricDetailView for the Fields/Series
   * pane, so both panes stay in lockstep without prop chains.
   */
  import {
    getMetricViewContext,
    type BucketSeriesError,
  } from '@/contexts/metric-view-context.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { selectionToQueryRangeMs } from '@/contexts/time-context.svelte'
  import { formatTimestamp } from '@/utils/time'
  import MetricTimeSeriesChart from '@/components/metrics/Charts/MetricTimeSeriesChart.svelte'
  import HistogramChart from '@/components/metrics/Charts/HistogramChart.svelte'
  import HistogramHeatmap from '@/components/metrics/Charts/HistogramHeatmap.svelte'
  import UnspecifiedTemporalityCallout from '@/components/metrics/Detail/UnspecifiedTemporalityCallout.svelte'
  import {
    DEFAULT_METRIC_CHART_HEIGHT,
    MIN_METRIC_CHART_HEIGHT,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import { CameraIcon, ChartHistogramIcon, TemperatureIcon } from '@/icons'

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  // Time window for the chart (used by histogram heatmap + aggregated
  // chart bounds). Re-derived here rather than read from the context
  // so the context doesn't need to expose a window field; this is
  // cheap.
  let queryRange = $derived(
    selectionToQueryRangeMs(timeContext.selection, Date.now())
  )

  /** Plot area inside the chart pane (flex slot below tabs / subtitle). */
  let plotHostHeight = $state(0)

  let plotHeight = $derived(
    plotHostHeight > 0
      ? Math.max(MIN_METRIC_CHART_HEIGHT, Math.floor(plotHostHeight))
      : DEFAULT_METRIC_CHART_HEIGHT
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
  <!-- Three tabs share this slot. The tab strip is fixed height; the
       plot host below flex-fills the chart pane and drives SVG height
       via bind:clientHeight. -->
  <div class="metric-chart-view metric-chart-view--fill">
    <div
      class="metric-chart-view__tabs"
      role="tablist"
      aria-label="Histogram views"
    >
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
    {#if ctx.activeHistogramTab === 'snapshot' && ctx.activeHistogramDp}
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
    {/if}
    <div
      class="metric-chart-view__plot-host"
      bind:clientHeight={plotHostHeight}
    >
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
              height={plotHeight}
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
              height={plotHeight}
            />
          {/if}
        {:else if ctx.activeHistogramTab === 'snapshot'}
          {#if ctx.activeHistogramDp}
            <HistogramChart
              datapoint={ctx.activeHistogramDp}
              unit={ctx.metric!.unit}
              quantileSource="datapoint"
              height={plotHeight}
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
  <div class="metric-chart-view metric-chart-view--fill">
    <div
      class="metric-chart-view__plot-host"
      bind:clientHeight={plotHostHeight}
    >
      <!-- transformedGaugeSumChartTimeseries is the post-view, already
           visibility-filtered set: the context applies the current
           AggregationView (Raw / Sum / Avg / Rate) and bucketing, then drops
           the timeseries the user has unchecked in the legend. The
           chart trusts this array and renders every entry; the parent
           also resolves the empty-state copy here, since only the
           parent can distinguish "no series at all" from "all
           unchecked" (the chart only sees the post-filter slice). -->
      <MetricTimeSeriesChart
        timeseries={ctx.transformedGaugeSumChartTimeseries}
        highlightedTimestamp={ctx.highlightedTimestamp}
        selectedSeriesKey={ctx.selectedSeriesKey}
        unit={ctx.metric!.unit}
        height={plotHeight}
        colorByKey={ctx.timeseriesColorByKey}
        aggregationView={ctx.aggregationView}
        emptyMessage={ctx.gaugeSumChartTimeseries.length === 0
          ? 'No datapoints to chart'
          : ctx.gaugeSumVisible.size === 0
            ? 'Nothing to see here — select a timeseries below'
            : 'No datapoints in selected timeseries'}
      />
    </div>
  </div>
{/snippet}

{#if !ctx.metric}
  <div class="metric-chart-view metric-chart-view--empty">
    <p class="text-base-content/40 text-sm">Select a metric to view details</p>
  </div>
{:else if ctx.isUnspecifiedTemporality}
  <!-- FunError takes the entire chart row. The detail pane (Fields /
       Series) still renders independently because it consumes
       different data; this branch only blanks the chart. -->
  <div class="metric-chart-view metric-chart-view--fill">
    <UnspecifiedTemporalityCallout size="full" />
  </div>
{:else if ctx.isHistogramKind}
  {@render histogramChartSlot()}
{:else if ctx.metricType === 'Gauge' || ctx.metricType === 'Sum'}
  {@render timeSeriesChartSlot()}
{:else}
  <div class="metric-chart-view metric-chart-view--fill">
    <div
      class="metric-chart-view__plot-host"
      bind:clientHeight={plotHostHeight}
    >
      <div class="metric-chart-view__placeholder">
        No chart available for this metric type
      </div>
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  /* Outer chart container. shrink-0 in the parent flex column so the
     chart keeps its requested height; the detail pane below claims
     the remaining vertical space. */
  .metric-chart-view {
    @apply shrink-0 p-2;
  }

  /* Histogram + time-series chart panes flex-grow into the chart pane
     so the SVG plot can be measured against the full available height.
     min-h-0 + overflow-hidden contains layerchart's measured size --
     without it a horizontal scrollbar at the bottom would push the
     wrapper taller than the pane and the measurement loop would
     escape the panel. */
  .metric-chart-view--fill {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
  }

  .metric-chart-view--empty {
    @apply flex h-full items-center justify-center;
  }

  .metric-chart-view__plot-host {
    @apply flex min-h-0 min-w-0 flex-1 flex-col;
  }

  .metric-chart-view__body {
    @apply flex min-h-0 min-w-0 flex-1 flex-col;
  }

  .metric-chart-view__placeholder {
    @apply flex min-h-0 flex-1 items-center justify-center text-base-content/40 text-sm;
  }

  .metric-chart-view__subtitle {
    @apply mb-1 mt-1 flex shrink-0 items-baseline gap-2 px-2;
  }

  .metric-chart-view__tabs {
    @apply flex w-full shrink-0 flex-nowrap items-center gap-1 px-1;

    & > :global(button) {
      flex: 1 1 0%;
      justify-content: center;
      padding-top: 0.25rem;
      padding-bottom: 0.25rem;
      font-size: 0.75rem;
    }
  }
</style>
