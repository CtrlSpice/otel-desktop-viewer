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
    />
  {/snippet}

  {#snippet meta()}
    <div class="flex w-full min-w-0 flex-col gap-2 pt-1">
      <span
        class="flex min-w-0 items-center gap-1 text-xs tabular-nums"
        title={`Last seen: ${lastSeenParts.value} ${lastSeenParts.unit}`}
      >
        <span class="shrink-0">Last seen:</span>
        <span class="truncate text-base-content">{lastSeenParts.value}</span>
        {#if lastSeenParts.unit}<span class="shrink-0"
            >{lastSeenParts.unit}</span
          >{/if}
      </span>
      <div
        class="flex min-w-0 items-center justify-between gap-3 text-xs tabular-nums"
      >
        {#if lastValueLabel || metric.unit}
          <span class="flex min-w-0 items-center gap-1">
            <span class="shrink-0"
              >{lastValueLabel ? 'Last value:' : 'Units:'}</span
            >
            <span
              class="truncate text-base-content"
              title={lastValueLabel ?? metric.unit}
            >
              {lastValueLabel ?? metric.unit}
            </span>
          </span>
        {/if}
        <span class="ml-auto flex shrink-0 items-center gap-1">
          <span>Datapoints:</span>
          <span class="text-base-content"
            >{metric.dataPointCount.toLocaleString()}</span
          >
        </span>
      </div>
    </div>
  {/snippet}
</SignalCard>
