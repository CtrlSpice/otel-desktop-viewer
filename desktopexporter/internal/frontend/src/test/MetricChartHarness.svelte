<script lang="ts">
  import { untrack, type Component } from 'svelte'
  import { createRouteContext } from '@/contexts/route-context.svelte'
  import { createTimeContext } from '@/contexts/time-context.svelte'
  import {
    createMetricViewContext,
    type MetricViewContext,
    type HistogramContributionSelection,
  } from '@/contexts/metric-view-context.svelte'
  import type { DataPoint, MetricViewData } from '@/types/api-types'

  type Props = {
    component: Component<any>
    componentProps?: Record<string, unknown>
    metric: MetricViewData
    seriesDatapoints?: Readonly<Record<string, DataPoint[]>>
    contribution?: HistogramContributionSelection | null
    oncontext?: (context: MetricViewContext) => void
  }

  let {
    component: TestComponent,
    componentProps = {},
    metric,
    seriesDatapoints,
    contribution,
    oncontext,
  }: Props = $props()

  createRouteContext()
  createTimeContext()
  const metricContext = createMetricViewContext(
    () => metric,
    () => null,
    () => null,
    () => null,
    seriesKey => seriesDatapoints?.[seriesKey],
    () => undefined,
    () => contribution ?? null
  )
  untrack(() => metricContext.seedForMetric(metric))
  untrack(() => oncontext?.(metricContext))
</script>

<TestComponent
  onClearSelection={metricContext.clearChartSelection}
  {...componentProps}
/>
