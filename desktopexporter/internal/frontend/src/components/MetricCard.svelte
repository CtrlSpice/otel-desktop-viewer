<script lang="ts">
  import type { MetricSummary } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import { metricTypeBadgeClass, metricTypeLabel } from '@/utils/metric-type'
  import SignalCard from '@/components/SignalCard.svelte'
  import MetricSparkline from '@/components/MetricCharts/MetricSparkline.svelte'
  import MetricSparkbars from '@/components/MetricCharts/MetricSparkbars.svelte'

  type Props = {
    metric: MetricSummary
    selected?: boolean
    onclick?: (id: string) => void
  }

  let { metric, selected = false, onclick }: Props = $props()

  let key = $derived(metricSummaryKey(metric))
  let isHistogramType = $derived(
    metric.metricType === 'Histogram' || metric.metricType === 'ExponentialHistogram'
  )
</script>

<SignalCard
  id={key}
  {selected}
  title={metric.name}
  subtitle={metric.serviceName || undefined}
  {onclick}
>
  {#snippet badge()}
    <span class={metricTypeBadgeClass(metric.metricType)}>
      {metricTypeLabel(metric.metricType)}
    </span>
  {/snippet}

  {#snippet spark()}
    {#if isHistogramType && metric.sparkbar}
      <MetricSparkbars buckets={metric.sparkbar} />
    {:else if metric.sparkline && metric.sparkline.length > 0}
      <MetricSparkline points={metric.sparkline} />
    {/if}
  {/snippet}
</SignalCard>
