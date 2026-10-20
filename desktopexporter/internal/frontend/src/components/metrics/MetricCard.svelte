<script lang="ts">
  import type { MetricSummary } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
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
    formatTimestampParts(metric.lastSeen, timeContext.tz, 'milliseconds')
  )

  let description = $derived((metric.description ?? '').trim())

  function formatLastValue(value: number): string {
    return new Intl.NumberFormat(undefined, {
      maximumFractionDigits: 6,
    }).format(value)
  }

  let lastValueLabel = $derived(
    metric.lastValue == null ? null : formatLastValue(metric.lastValue)
  )
</script>

<SignalCard
  id={key}
  {selected}
  title={metric.name}
  subtitle={metric.serviceName?.trim() || undefined}
  description={description || undefined}
  {onclick}
>
  {#snippet badge()}
    <SignalBadges
      signal="metric"
      metricType={metric.metricType}
      aggregationTemporality={metric.aggregationTemporality}
      isMonotonic={metric.isMonotonic}
      seriesCount={metric.seriesCount}
      seriesCardinality={metric.seriesCardinality}
      dataPointCount={metric.dataPointCount}
    />
  {/snippet}

  {#snippet date()}
    <span
      class="flex min-w-0 items-center gap-1"
      title={`Last seen: ${lastSeenParts.value} ${lastSeenParts.unit}`}
    >
      <span class="shrink-0">Last seen:</span>
      <span class="truncate text-base-content">{lastSeenParts.value}</span>
    </span>
  {/snippet}

  {#snippet meta()}
    <div
      class="grid w-full min-w-0 grid-cols-2 gap-x-4 gap-y-1.5 text-xs tabular-nums"
    >
      {#if lastValueLabel}
        <span class="flex min-w-0 items-center gap-1">
          <span class="shrink-0">Last value:</span>
          <span class="truncate text-base-content" title={lastValueLabel}>
            {lastValueLabel}
          </span>
        </span>
      {/if}
      {#if metric.unit}
        <span class="flex min-w-0 items-center gap-1">
          <span class="shrink-0">Units:</span>
          <span class="truncate text-base-content" title={metric.unit}
            >{metric.unit}</span
          >
        </span>
      {/if}
    </div>
  {/snippet}
</SignalCard>
