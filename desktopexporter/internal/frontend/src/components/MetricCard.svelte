<script lang="ts">
  import type { MetricData } from '@/types/api-types'
  import { getServiceName } from '@/utils/resource'
  import { metricTypeBadgeClass, metricTypeLabel } from '@/utils/metric-type'
  import SignalCard from '@/components/SignalCard.svelte'
  import MetricSparkline from '@/components/MetricCharts/MetricSparkline.svelte'
  import MetricSparkbars from '@/components/MetricCharts/MetricSparkbars.svelte'

  type Props = {
    metric: MetricData
    selected?: boolean
    onclick?: (id: string) => void
  }

  let { metric, selected = false, onclick }: Props = $props()

  let metricType = $derived(metric.datapoints[0]?.metricType ?? 'Empty')
  let service = $derived(getServiceName(metric.resource) ?? '')
  let isHistogramType = $derived(metricType === 'Histogram' || metricType === 'ExponentialHistogram')
</script>

<SignalCard
  id={metric.id}
  {selected}
  title={metric.name}
  subtitle={service || undefined}
  {onclick}
>
  {#snippet badge()}
    <span class={metricTypeBadgeClass(metricType)}>{metricTypeLabel(metricType)}</span>
  {/snippet}

  {#snippet spark()}
    {#if isHistogramType}
      <MetricSparkbars datapoints={metric.datapoints} />
    {:else if metricType === 'Gauge' || metricType === 'Sum'}
      <MetricSparkline datapoints={metric.datapoints} />
    {/if}
  {/snippet}
</SignalCard>
