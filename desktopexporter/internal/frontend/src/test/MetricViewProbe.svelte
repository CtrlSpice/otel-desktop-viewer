<script lang="ts">
  import { untrack } from 'svelte'
  import {
    createMetricViewContext,
    type MetricViewContext,
  } from '@/contexts/metric-view-context.svelte'
  import type {
    DataPoint,
    MetricData,
    ScalarAggregate,
  } from '@/types/api-types'
  import TimeseriesPanel from '@/components/metrics/Detail/TimeseriesPanel.svelte'

  // createMetricViewContext() registers $effects, so it only works inside a
  // component. This probe renders the sub-view state the URL sync owns so
  // tests can observe it through the DOM, and hands the context back so
  // tests can drive it through its public methods.
  type Props = {
    metric: MetricData | undefined
    seriesDatapoints?: Readonly<Record<string, DataPoint[]>>
    scalarAggregate?: ScalarAggregate | null
    oncontext?: (ctx: MetricViewContext) => void
    showTimeseries?: boolean
  }
  let {
    metric,
    seriesDatapoints,
    scalarAggregate,
    oncontext,
    showTimeseries,
  }: Props = $props()

  const metricCtx = createMetricViewContext(
    () => metric,
    () => null,
    () => null,
    () => scalarAggregate ?? null,
    seriesKey => seriesDatapoints?.[seriesKey]
  )
  // The page seeds synchronously in the same statement that assigns the
  // metric; the probe stands in for that. Seeding is no longer an effect, so
  // without this the sub-view defaults are never applied.
  untrack(() => metricCtx.seedForMetric(metric))
  // Handing the context back is a one-time setup step, not a subscription.
  untrack(() => oncontext?.(metricCtx))
</script>

{#if showTimeseries}
  <TimeseriesPanel />
{/if}

<output data-testid="aggregation-view">{metricCtx.aggregationView}</output>
<output data-testid="selected-datapoint-id"
  >{metricCtx.selectedDatapointID ?? ''}</output
>
<output data-testid="selected-datapoint-resolved-id"
  >{metricCtx.selectedDatapoint?.id ?? ''}</output
>
<output data-testid="selected-series-key"
  >{metricCtx.selectedSeriesKey ?? ''}</output
>
<output data-testid="available-aggregation-views"
  >{metricCtx.availableAggregationViews.join(',')}</output
>
<output data-testid="chart-time-start"
  >{metricCtx.chartDataTimeRange?.startMs ?? ''}</output
>
<output data-testid="chart-time-end"
  >{metricCtx.chartDataTimeRange?.endMs ?? ''}</output
>
