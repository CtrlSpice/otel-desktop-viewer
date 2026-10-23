<script lang="ts">
  import type { MetricSummary } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import ServiceDot from '@/components/shared/ServiceDot.svelte'
  import { metricTypeSummary } from './utils/metric-type'

  type Props = {
    metric: MetricSummary
    selected?: boolean
    onclick?: (id: string) => void
    href?: string
    matchingPointCount?: number
  }

  let {
    metric,
    selected = false,
    onclick,
    href,
    matchingPointCount,
  }: Props = $props()

  let key = $derived(metricSummaryKey(metric))

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
  meta={lastValueLabel !== null || matchingPointCount !== undefined
    ? reading
    : undefined}
  {onclick}
  {href}
>
  {#snippet subtitleContent()}
    <div class="metric-card__classification">
      {#if metric.serviceName?.trim()}
        <ServiceDot service={metric.serviceName} />
        <span class="min-w-0 truncate text-rp-subtle" title={metric.serviceName}
          >{metric.serviceName.trim()}</span
        >
      {/if}
      <span class="ml-auto shrink-0 text-secondary" title={typeSummary.title}
        >{metric.metricType === 'ExponentialHistogram'
          ? 'Exp. Histogram'
          : metric.metricType}</span
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
    <SignalDate label="Last seen" timestamp={metric.lastSeen} />
  {/snippet}
</SignalCard>

{#snippet reading()}
  <div class="flex w-full min-w-0 flex-wrap gap-x-2 text-sm tabular-nums">
    {#if matchingPointCount !== undefined}<span class="text-rp-subtle"
        >[{matchingPointCount.toLocaleString()} matching {matchingPointCount ===
        1
          ? 'datapoint'
          : 'datapoints'}]</span
      >{/if}
    {#if lastValueLabel}
      <span class="flex min-w-0 items-center gap-1">
        <span class="shrink-0">Last value:</span>
        <span class="truncate text-base-content" title={lastValueLabel}>
          {lastValueLabel}
        </span>
        {#if metric.unit}<span class="text-base-content">{metric.unit}</span
          >{/if}
      </span>
    {/if}
  </div>
{/snippet}

<style lang="postcss">
  @reference "../../app.css";

  .metric-card__classification {
    @apply flex min-w-0 items-center text-sm;
    min-height: var(--signal-card-row-h);
    gap: var(--header-gap);
  }
</style>
