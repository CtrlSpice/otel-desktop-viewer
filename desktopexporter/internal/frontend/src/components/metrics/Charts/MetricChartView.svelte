<script lang="ts">
  /*
   * MetricChartView is the "main" pane on the metrics page. It owns
   * the chart surface (histogram views, time-series chart, or an
   * unspecified-temporality callout). Histogram view tabs live in the
   * metrics page PaneHeader, same as Gauge/Sum aggregation tabs.
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
  import MetricTimeSeriesChart from '@/components/metrics/Charts/MetricTimeSeriesChart.svelte'
  import MetricQuantileAreaChart from '@/components/metrics/Charts/MetricQuantileAreaChart.svelte'
  import HistogramChart from '@/components/metrics/Charts/HistogramChart.svelte'
  import HistogramHeatmap from '@/components/metrics/Charts/HistogramHeatmap.svelte'
  import UnspecifiedTemporalityCallout from '@/components/metrics/Detail/UnspecifiedTemporalityCallout.svelte'
  import {
    DEFAULT_METRIC_CHART_HEIGHT,
    MIN_METRIC_CHART_HEIGHT,
    chartPadding,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import MetricChartControlBar from '@/components/metrics/Charts/MetricChartControlBar.svelte'
  import HistogramQuantileControlBar from '@/components/metrics/Charts/HistogramQuantileControlBar.svelte'
  import HistogramHeatmapControlBar from '@/components/metrics/Charts/HistogramHeatmapControlBar.svelte'
  import HistogramChartControlBar from '@/components/metrics/Charts/HistogramChartControlBar.svelte'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import { formatDateTime } from '@/utils/time'
  import { QUANTILE_LABELS } from '@/components/metrics/utils/histogram-aggregation'

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  let activeQuantileKeys = $derived(
    QUANTILE_LABELS.map(q => q.key).filter(k => ctx.activeQuantileOverlays.has(k))
  )

  let histogramBucketTimestamp = $derived.by((): string => {
    if (ctx.histogramScope !== 'bucket') return ''
    const dp = ctx.histogramChartDatapoint
    if (!dp) return ''
    return formatDateTime(
      Number(dp.timestamp / 1_000_000n),
      timeContext.tz,
      'milliseconds'
    )
  })

  let histogramEmptyMessage = $derived.by((): string => {
    if (ctx.histogramScope === 'bucket') {
      return 'Select a datapoint'
    }
    return 'No aggregate in range'
  })

  let quantileEmptyMessage = $derived.by((): string => {
    if (ctx.bucketSeriesError) return 'Cannot chart quantiles for this metric'
    if (ctx.histogramTimeseriesCount === 0) return 'No datapoints to chart'
    if (ctx.histogramVisible.size === 0) {
      return 'Nothing to see here — select a timeseries below'
    }
    return 'No quantile data in range'
  })

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
  <div class="metric-chart-view metric-chart-view--fill">
    <div
      class="metric-chart-view__plot-host"
      bind:clientHeight={plotHostHeight}
    >
      {#if ctx.activeHistogramTab === 'heatmap'}
        {#if ctx.bucketSeriesError}
          {@render bucketSeriesErrorMessage(ctx.bucketSeriesError)}
        {:else if ctx.heatmapBucketSeries === null}
          <div class="metric-chart-view__placeholder">No histogram data</div>
        {:else if ctx.heatmapBucketSeries.length === 0}
          <MetricChartEmpty height={plotHeight} message="No bucket data in range" />
        {:else}
          <HistogramHeatmap
            points={ctx.heatmapBucketSeries}
            height={plotHeight}
            unit={ctx.metric!.unit}
            timeRange={ctx.chartDataTimeRange ?? null}
            plotPaddingBottom={chartPadding.bottom}
            onSelect={ctx.onHeatmapSelect}
            selectedTimestamp={ctx.heatmapSelectedTimestamp}
          />
        {/if}
      {:else if ctx.activeHistogramTab === 'quantiles'}
        {#if ctx.bucketSeriesError}
          {@render bucketSeriesErrorMessage(ctx.bucketSeriesError)}
        {:else if ctx.quantileChartTimeseries.length === 0}
          <MetricChartEmpty height={plotHeight} message={quantileEmptyMessage} />
        {:else}
          <MetricQuantileAreaChart
            timeseries={ctx.quantileChartTimeseries}
            activeQuantileKeys={activeQuantileKeys}
            unit={ctx.metric!.unit}
            height={plotHeight}
            timeRange={ctx.chartDataTimeRange ?? null}
            colorByKey={ctx.timeseriesColorByKey}
            onChartPointClick={ctx.onQuantileChartPointClick}
            emptyMessage={quantileEmptyMessage}
          />
        {/if}
      {:else if ctx.activeHistogramTab === 'histogram'}
        {#if ctx.histogramChartError}
          {@render bucketSeriesErrorMessage(ctx.histogramChartError)}
        {:else if !ctx.histogramChartDatapoint}
          <MetricChartEmpty height={plotHeight} message={histogramEmptyMessage} />
        {:else}
          <HistogramChart
            datapoint={ctx.histogramChartDatapoint}
            unit={ctx.metric!.unit}
            height={plotHeight}
            timeRange={ctx.histogramScope === 'window'
              ? (ctx.chartDataTimeRange ?? null)
              : null}
            selectionTimestamp={histogramBucketTimestamp}
            enableValueBucketPin={ctx.histogramScope === 'window'}
          />
        {/if}
      {/if}
    </div>
    {#if ctx.activeHistogramTab === 'quantiles'}
      <HistogramQuantileControlBar />
    {:else if ctx.activeHistogramTab === 'heatmap'}
      <HistogramHeatmapControlBar />
    {:else if ctx.activeHistogramTab === 'histogram'}
      <HistogramChartControlBar />
    {/if}
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
        showStatOverlays={ctx.showSelectionStatOverlays}
        selectedRateSlope={ctx.selectedRateSlope}
        timeRange={ctx.chartDataTimeRange ?? null}
        onChartPointClick={ctx.onChartPointClick}
        emptyMessage={ctx.gaugeSumChartTimeseries.length === 0
          ? 'No datapoints to chart'
          : ctx.gaugeSumVisible.size === 0
            ? 'Nothing to see here — select a timeseries below'
            : 'No datapoints in selected timeseries'}
      />
    </div>
    <MetricChartControlBar />
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

  .metric-chart-view__placeholder {
    @apply flex min-h-0 flex-1 items-center justify-center text-base-content/40 text-sm;
  }
</style>
