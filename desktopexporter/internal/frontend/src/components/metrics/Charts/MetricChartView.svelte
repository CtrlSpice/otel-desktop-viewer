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
  import HistogramChart from '@/components/metrics/Charts/HistogramChart.svelte'
  import HistogramHeatmap from '@/components/metrics/Charts/HistogramHeatmap.svelte'
  import UnspecifiedTemporalityCallout from '@/components/metrics/Detail/UnspecifiedTemporalityCallout.svelte'
  import {
    DEFAULT_METRIC_CHART_HEIGHT,
    MIN_METRIC_CHART_HEIGHT,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import MetricChartControlBar from '@/components/metrics/Charts/MetricChartControlBar.svelte'
  import HistogramQuantileControlBar from '@/components/metrics/Charts/HistogramQuantileControlBar.svelte'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import ChartTimeRangeHeader from '@/components/metrics/Charts/ChartTimeRangeHeader.svelte'
  import ChartSelectionLegend from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
  import { formatDateTime } from '@/utils/time'

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  function onQuantileChartKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return
    if (ctx.activeHistogramTab !== 'quantiles') return
    if (!ctx.quantileDrillDownActive) return
    ctx.clearQuantileDrillDown()
  }

  let quantileHighlightedTimestamp = $derived.by((): bigint | null => {
    if (ctx.activeHistogramTab !== 'quantiles') return null
    const ms = ctx.heatmapSelectedTimestamp
    if (ms === null) return null
    return BigInt(ms) * 1_000_000n
  })

  let snapshotSelectionTimestamp = $derived.by((): string => {
    if (ctx.activeHistogramTab !== 'snapshot') return ''
    const dp = ctx.activeHistogramDp
    if (!dp) return ''
    return formatDateTime(
      Number(dp.timestamp / 1_000_000n),
      timeContext.timezone,
      'milliseconds'
    )
  })

  let quantileEmptyMessage = $derived.by((): string => {
    if (ctx.bucketSeriesError) return 'Cannot chart quantiles for this metric'
    if (ctx.histogramTimeseriesCount === 0) return 'No datapoints to chart'
    if (ctx.histogramVisible.size === 0) {
      return 'Nothing to see here — select a timeseries below'
    }
    if (ctx.activeQuantileOverlays.size === 0) {
      return 'Enable at least one quantile overlay'
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

<svelte:window onkeydown={onQuantileChartKeydown} />

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
      {#if ctx.chartDataTimeRange || snapshotSelectionTimestamp}
        <div class="metric-chart-view__header">
          {#if ctx.chartDataTimeRange}
            <ChartTimeRangeHeader
              startMs={ctx.chartDataTimeRange.startMs}
              endMs={ctx.chartDataTimeRange.endMs}
              variant="legend"
            />
          {/if}
          {#if snapshotSelectionTimestamp}
            <div class="metric-chart-view__selection-legend">
              <ChartSelectionLegend
                timestamp={snapshotSelectionTimestamp}
                rows={[]}
              />
            </div>
          {/if}
        </div>
      {/if}
      <div class="metric-chart-view__body">
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
              windowStartMs={queryRange.start}
              windowEndMs={queryRange.end}
              height={plotHeight}
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
            <MetricTimeSeriesChart
              timeseries={ctx.quantileChartTimeseries}
              highlightedTimestamp={quantileHighlightedTimestamp}
              unit={ctx.metric!.unit}
              height={plotHeight}
              colorByKey={ctx.quantileColorByKey}
              aggregationView="raw"
              showStatOverlays={false}
              onChartPointClick={ctx.onQuantileChartPointClick}
              emptyMessage={quantileEmptyMessage}
              useQuantileLineStyle={true}
            />
          {/if}
        {:else if ctx.activeHistogramTab === 'aggregated'}
          {#if ctx.aggregatedError}
            {@render bucketSeriesErrorMessage(ctx.aggregatedError)}
          {:else if !ctx.aggregatedDatapoint}
            <div class="metric-chart-view__placeholder">No aggregate in range</div>
          {:else}
            <HistogramChart
              datapoint={ctx.aggregatedDatapoint}
              unit={ctx.metric!.unit}
              height={plotHeight}
            />
          {/if}
        {:else if ctx.activeHistogramTab === 'snapshot'}
          {#if ctx.activeHistogramDp}
            <HistogramChart
              datapoint={ctx.activeHistogramDp}
              unit={ctx.metric!.unit}
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
    {#if ctx.activeHistogramTab === 'quantiles'}
      <HistogramQuantileControlBar />
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

  .metric-chart-view__header {
    @apply flex shrink-0 items-start justify-between gap-2 px-1 pb-1 pt-0.5;
  }

  .metric-chart-view__selection-legend {
    @apply ml-auto shrink-0;
    pointer-events: none;
  }

  .metric-chart-view__body {
    @apply flex min-h-0 min-w-0 flex-1 flex-col;
  }

  .metric-chart-view__placeholder {
    @apply flex min-h-0 flex-1 items-center justify-center text-base-content/40 text-sm;
  }
</style>
