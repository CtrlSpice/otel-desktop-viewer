<script lang="ts">
  import type { MetricSummary } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import { metricTypeCardBadge } from '@/utils/metric-type'
  import SignalCard from '@/components/SignalCard.svelte'
  import { formatTimestampParts } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    metric: MetricSummary
    selected?: boolean
    onclick?: (id: string) => void
  }

  let { metric, selected = false, onclick }: Props = $props()

  const timeContext = getTimeContext()

  let key = $derived(metricSummaryKey(metric))

  let lastSeenParts = $derived(
    formatTimestampParts(
      metric.lastSeen,
      timeContext.timezone,
      'milliseconds'
    )
  )

  let typeBadge = $derived(
    metricTypeCardBadge(
      metric.metricType,
      metric.aggregationTemporality,
      metric.isMonotonic
    )
  )

  let description = $derived((metric.description ?? '').trim())

  function formatLastValue(value: number): string {
    return new Intl.NumberFormat(undefined, {
      maximumFractionDigits: 6,
    }).format(value)
  }

  let lastValueLabel = $derived.by(() => {
    if (metric.lastValue == null) return null
    const value = formatLastValue(metric.lastValue)
    const unit = metric.unit?.trim()
    return unit ? `${value} ${unit}` : value
  })
</script>

<SignalCard
  id={key}
  {selected}
  title={metric.name}
  subtitle={metric.serviceName?.trim() || undefined}
  description={description || undefined}
  timeLayout="labeled"
  timestampLabel="Last seen:"
  timestamp={lastSeenParts.value}
  timestampUnit={lastSeenParts.unit || undefined}
  {onclick}
>
  {#snippet badge()}
    <span class={typeBadge.className} title={typeBadge.title}>
      {typeBadge.label}
    </span>
    <span
      class="badge badge-xs badge-soft badge-neutral tabular-nums"
      title="{metric.seriesCount} time series in range"
    >
      {metric.seriesCount} series
    </span>
  {/snippet}

  {#snippet meta()}
    {#if lastValueLabel}
      <span class="metric-card__labeled">
        <span class="signal-row__label">Last value:</span>
        <span class="metric-card__labeled-value">{lastValueLabel}</span>
      </span>
    {/if}
    {#if metric.unit && metric.lastValue == null}
      <span class="metric-card__labeled">
        <span class="signal-row__label">Units:</span>
        <span class="metric-card__labeled-value">{metric.unit}</span>
      </span>
    {/if}
  {/snippet}
</SignalCard>

<style lang="postcss">
  @reference "../app.css";

  .metric-card__labeled {
    @apply inline-flex min-w-0 items-center gap-x-1;
  }

  .metric-card__labeled-value {
    @apply truncate leading-none text-base-content;
  }
</style>
