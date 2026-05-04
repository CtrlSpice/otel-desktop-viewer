<script lang="ts">
  import type { MetricSummary } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import { metricTypeBadgeClass, metricTypeLabel, metricTypeSeriesColor } from '@/utils/metric-type'
  import SignalCard from '@/components/SignalCard.svelte'
  import MetricSparkline from '@/components/MetricCharts/MetricSparkline.svelte'
  import MetricSparkbars from '@/components/MetricCharts/MetricSparkbars.svelte'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    metric: MetricSummary
    selected?: boolean
    onclick?: (id: string) => void
  }

  let { metric, selected = false, onclick }: Props = $props()

  const timeContext = getTimeContext()

  let key = $derived(metricSummaryKey(metric))
  let isHistogramType = $derived(
    metric.metricType === 'Histogram' ||
      metric.metricType === 'ExponentialHistogram'
  )

  let receivedLabel = $derived(
    formatTimestamp(metric.received, timeContext.timezone, 'milliseconds')
  )

  let sparkColor = $derived(metricTypeSeriesColor(metric.metricType))
</script>

<SignalCard
  id={key}
  {selected}
  title={metric.name}
  subtitle={metric.serviceName || undefined}
  timestamp={receivedLabel}
  {onclick}
>
  {#snippet badge()}
    <span class={metricTypeBadgeClass(metric.metricType)}>
      {metricTypeLabel(metric.metricType)}
    </span>
  {/snippet}

  {#snippet meta()}
    {#if metric.unit}
      <span class="tabular-nums text-base-content/40" title={metric.unit}>
        units: {metric.unit}
      </span>
    {/if}
  {/snippet}

  {#snippet spark()}
    {#if isHistogramType && metric.sparkbar}
      <MetricSparkbars buckets={metric.sparkbar} seriesColor={sparkColor} />
    {:else if metric.sparkline && metric.sparkline.length > 0}
      <MetricSparkline points={metric.sparkline} seriesColor={sparkColor} />
    {/if}
  {/snippet}
</SignalCard>
