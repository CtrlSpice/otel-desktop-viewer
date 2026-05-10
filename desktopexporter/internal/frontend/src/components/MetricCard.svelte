<script lang="ts">
  import type { MetricSummary } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import { metricTypeBadgeClass, metricTypeLabel, metricTypeSeriesColor } from '@/utils/metric-type'
  import SignalCard from '@/components/SignalCard.svelte'
  import MetricSparkline from '@/components/MetricCharts/MetricSparkline.svelte'
  import MetricSparkbars from '@/components/MetricCharts/MetricSparkbars.svelte'
  import UnspecifiedTemporalityCallout from '@/components/MetricDetails/UnspecifiedTemporalityCallout.svelte'
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

  // The spark slot reads one of two SparkOutcome unions depending on metric
  // type. Pattern-matching on `kind` lets the template stay flat (no nested
  // ternaries / guard chains) and gives us a single uniform "error" branch
  // for whichever FunError reason the backend sent.
  let activeSpark = $derived(isHistogramType ? metric.sparkbar : metric.sparkline)
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
    <!-- 96px keeps a 5-bucket histogram from smearing into chunky stripes
         and a 20-point sparkline from going lazy/curvy. The width lives
         here (not in SignalCard) because logs/traces won't necessarily
         want the same box. -->
    <div class="metric-card__spark-box">
      {#if activeSpark?.kind === 'error'}
        {#if activeSpark.reason === 'unspecifiedTemporality'}
          <UnspecifiedTemporalityCallout size="mini" />
        {/if}
      {:else if isHistogramType && metric.sparkbar?.kind === 'data'}
        <MetricSparkbars buckets={metric.sparkbar.value} seriesColor={sparkColor} />
      {:else if metric.sparkline?.kind === 'data' && metric.sparkline.value.length > 0}
        <MetricSparkline points={metric.sparkline.value} seriesColor={sparkColor} />
      {/if}
    </div>
  {/snippet}
</SignalCard>

<style lang="postcss">
  @reference "../app.css";

  .metric-card__spark-box {
    @apply h-full w-24;
  }
</style>
