<script lang="ts">
  import type { MetricSummary } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import { formatTimestampParts } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { metricTypeSummary } from './utils/metric-type'

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
  let typeSummary = $derived(
    metricTypeSummary(
      metric.metricType,
      metric.aggregationTemporality,
      metric.isMonotonic
    )
  )

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
  description={description || undefined}
  {onclick}
>
  {#snippet subtitleContent()}
    <div class="metric-card__classification">
      {#if metric.serviceName?.trim()}
        <span class="min-w-0 truncate text-rp-subtle" title={metric.serviceName}
          >{metric.serviceName.trim()}</span
        >
      {/if}
      <span class="ml-auto shrink-0 text-secondary" title={typeSummary.title}
        >{typeSummary.label}</span
      >
    </div>
  {/snippet}
  {#snippet badge()}
    <SignalBadges
      signal="metric"
      metricType={metric.metricType}
      aggregationTemporality={metric.aggregationTemporality}
      isMonotonic={metric.isMonotonic}
      showType={false}
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
    <div class="grid w-full min-w-0 grid-cols-2 gap-x-4 text-xs tabular-nums">
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

<style lang="postcss">
  @reference "../../app.css";

  .metric-card__classification {
    @apply flex min-w-0 items-center text-sm;
    min-height: var(--signal-card-row-h);
    gap: var(--header-gap);
  }
</style>
