<script lang="ts">
  import { untrack } from 'svelte'
  import {
    createMetricViewContext,
    type MetricViewContext,
  } from '@/contexts/metric-view-context.svelte'
  import type { MetricData } from '@/types/api-types'

  // createMetricViewContext() runs $effect, so it only works inside a
  // component. This probe renders the sub-view state the URL sync owns so
  // tests can observe it through the DOM, and hands the context back so
  // tests can drive it through its public methods.
  type Props = {
    metric: MetricData | undefined
    oncontext?: (ctx: MetricViewContext) => void
  }
  let { metric, oncontext }: Props = $props()

  const metricCtx = createMetricViewContext(() => metric)
  // Handing the context back is a one-time setup step, not a subscription.
  untrack(() => oncontext?.(metricCtx))
</script>

<output data-testid="aggregation-view">{metricCtx.aggregationView}</output>
<output data-testid="selected-datapoint-id"
  >{metricCtx.selectedDatapointId ?? ''}</output
>
<output data-testid="available-aggregation-views"
  >{metricCtx.availableAggregationViews.join(',')}</output
>
