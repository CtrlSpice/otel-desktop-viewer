<script lang="ts" generics="TComponent extends Component<any>">
  import { untrack, type Component, type ComponentProps } from 'svelte'
  import { createRouteContext } from '@/contexts/route-context.svelte'
  import { createTimeContext } from '@/contexts/time-context.svelte'
  import {
    createMetricViewContext,
    type MetricViewContext,
  } from '@/contexts/metric-view-context.svelte'
  import type { DataPoint, MetricData } from '@/types/api-types'

  type Props = {
    component: TComponent
    componentProps: ComponentProps<TComponent>
    metric: MetricData
    seriesDatapoints?: Readonly<Record<string, DataPoint[]>>
    oncontext?: (context: MetricViewContext) => void
  }

  let {
    component: TestComponent,
    componentProps,
    metric,
    seriesDatapoints,
    oncontext,
  }: Props = $props()

  createRouteContext()
  createTimeContext()
  const metricContext = createMetricViewContext(
    () => metric,
    () => null,
    () => null,
    () => null,
    seriesKey => seriesDatapoints?.[seriesKey]
  )
  untrack(() => metricContext.seedForMetric(metric))
  untrack(() => oncontext?.(metricContext))
</script>

<TestComponent
  onClearSelection={metricContext.clearChartSelection}
  {...componentProps}
/>
