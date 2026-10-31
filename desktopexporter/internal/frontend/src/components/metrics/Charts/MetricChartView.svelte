<script lang="ts">
  /*
   * MetricChartView is the "main" pane on the metrics page. It owns
   * the chart surface (combined histogram/quantile views, time-series chart, or an
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
  import MetricTimeSeriesChart from '@/components/metrics/Charts/MetricTimeSeriesChart.svelte'
  import HistogramChart from '@/components/metrics/Charts/HistogramChart.svelte'
  import HistogramHeatmap from '@/components/metrics/Charts/HistogramHeatmap.svelte'
  import HistogramCountLegend from '@/components/metrics/Charts/HistogramCountLegend.svelte'
  import UnspecifiedTemporalityCallout from '@/components/metrics/Detail/UnspecifiedTemporalityCallout.svelte'
  import {
    DEFAULT_METRIC_CHART_HEIGHT,
    MIN_METRIC_CHART_HEIGHT,
    chartPadding,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import type { MetricViewData } from '@/types/api-types'

  let {
    aggregateLoading = false,
    aggregateSummaryLoading = false,
    aggregateError = '',
    aggregateSummaryError = '',
    aggregateMetric,
    aggregateSummaryMetric,
    requestKey,
  }: {
    aggregateLoading?: boolean
    aggregateSummaryLoading?: boolean
    aggregateError?: string
    aggregateSummaryError?: string
    aggregateMetric?: MetricViewData
    aggregateSummaryMetric?: MetricViewData
    requestKey?: AbortSignal
  } = $props()

  const ctx = getMetricViewContext()
  const pending = $derived(
    ctx.isHistogramKind && ctx.activeHistogramTab === 'histogram'
      ? aggregateSummaryLoading
      : aggregateLoading
  )
  const error = $derived(
    ctx.isHistogramKind && ctx.activeHistogramTab === 'histogram'
      ? aggregateSummaryError
      : aggregateError
  )

  /** Plot area inside the chart pane (flex slot below tabs / subtitle). */
  let plotHostHeight = $state(0)

  let plotHeight = $derived(
    plotHostHeight > 0
      ? Math.max(MIN_METRIC_CHART_HEIGHT, Math.floor(plotHostHeight))
      : DEFAULT_METRIC_CHART_HEIGHT
  )

  function histogramEmptyMessage(hasIntervals: boolean) {
    if (ctx.visibleSeries.size === 0)
      return 'Nothing to see here — select a timeseries below'
    const hasReadings = ctx.metric?.timeseries.some(
      series =>
        ctx.visibleSeries.has(series.seriesRef) && series.datapointCount > 0
    )
    if (ctx.temporalityCode === 2 && hasReadings) {
      return hasIntervals
        ? 'No interval observations in this range. Cumulative charts show changes between readings; recorded totals are available in Details.'
        : 'Not enough cumulative readings to calculate interval activity. Recorded totals are available in Details.'
    }
    return hasIntervals
      ? 'No observations in this range'
      : 'No bucket data in range'
  }
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
    <HistogramCountLegend />
    <div
      class="metric-chart-view__plot-host"
      bind:clientHeight={plotHostHeight}
    >
      {#if ctx.activeHistogramTab === 'heatmap'}
        {#if ctx.bucketSeriesError}
          {@render bucketSeriesErrorMessage(ctx.bucketSeriesError)}
        {:else if (pending || error) && !ctx.heatmapBucketSeries?.length}
          <div class="metric-chart-view__placeholder"></div>
        {:else if ctx.heatmapBucketSeries === null}
          <div class="metric-chart-view__placeholder">No histogram data</div>
        {:else if ctx.heatmapBucketSeries.length === 0 || ctx.heatmapBucketSeries.every(point => point.totals.count === 0)}
          <MetricChartEmpty
            height={plotHeight}
            message={histogramEmptyMessage(ctx.heatmapBucketSeries.length > 0)}
          />
        {:else}
          <HistogramHeatmap
            points={ctx.heatmapBucketSeries}
            height={plotHeight}
            unit={(aggregateMetric ?? ctx.metric)!.unit}
            timeRange={ctx.chartDataTimeRange ?? null}
            plotPaddingBottom={chartPadding.bottom}
            onSelect={ctx.onHeatmapSelect}
            onClearSelection={ctx.clearChartSelection}
            selectedTimestamp={ctx.heatmapColumnStartNs}
          />
        {/if}
      {:else if ctx.activeHistogramTab === 'histogram'}
        {#if ctx.histogramChartError}
          {@render bucketSeriesErrorMessage(ctx.histogramChartError)}
        {:else if (pending || error) && !ctx.histogramChartDatapoint}
          <div class="metric-chart-view__placeholder"></div>
        {:else if !ctx.histogramChartDatapoint || ctx.histogramChartDatapoint.count === 0}
          <MetricChartEmpty
            height={plotHeight}
            message={histogramEmptyMessage(
              Boolean(ctx.histogramChartDatapoint)
            )}
          />
        {:else}
          <HistogramChart
            datapoint={ctx.histogramChartDatapoint}
            unit={(aggregateSummaryMetric ?? ctx.metric)!.unit}
            height={plotHeight}
            onClearSelection={ctx.clearChartSelection}
          />
        {/if}
      {/if}
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
        highlightedPointID={ctx.selectedDatapointID}
        selectedSeriesKey={ctx.selectedSeriesKey}
        previewSeriesKey={ctx.previewSeriesKey}
        unit={ctx.metric!.unit}
        height={plotHeight}
        colorByKey={ctx.timeseriesColorByKey}
        aggregationView={ctx.aggregationView}
        showStatOverlays={ctx.showSelectionStatOverlays}
        timeRange={ctx.chartDataTimeRange ?? null}
        onChartPointClick={ctx.onChartPointClick}
        onRateChartPointClick={ctx.onRateChartPointClick}
        onClearSelection={ctx.clearChartSelection}
        seriesStats={ctx.seriesStatsByKey}
        emptyMessage={ctx.gaugeSumSeriesKeys.length === 0
          ? 'No datapoints to chart'
          : ctx.visibleSeries.size === 0
            ? 'Nothing to see here — select a timeseries below'
            : 'No datapoints in selected timeseries'}
      />
    </div>
  </div>
{/snippet}

{#key ctx.isHistogramKind ? ctx.activeHistogramTab : 'scalar'}
  <LoadingStatus
    {pending}
    {requestKey}
    label="Loading chart aggregate…"
    class="block p-3 text-subtle"
  />
{/key}
{#if error}<p role="alert" class="p-3 text-papaya">{error}</p>{/if}
{#if !ctx.metric}
  <div class="metric-chart-view metric-chart-view--empty">
    <p class="text-base-content/40 text-sm">Select a metric to view details</p>
  </div>
{:else if ctx.isUnsafeTemporality}
  <!-- FunError takes the entire chart row. The detail pane (Fields /
       Series) still renders independently because it consumes
       different data; this branch only blanks the chart. -->
  <div class="metric-chart-view metric-chart-view--fill">
    <UnspecifiedTemporalityCallout
      size="full"
      temporalityCode={ctx.temporalityCode!}
      temporalityLabel={ctx.temporality}
    />
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
