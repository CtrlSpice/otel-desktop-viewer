<script module lang="ts">
  /* Shared drawer-card and detail-header badges from primitive signal facts. */
  import type { MetricType } from '@/types/api-types'
  import { metricTypeCardBadge } from '@/components/metrics/utils/metric-type'
  import {
    severityBadgeClass,
    severityLabel,
  } from '@/components/logs/log-severity'

  type MetricProps = {
    signal: 'metric'
    metricType: MetricType | string
    aggregationTemporality: string | null | undefined
    isMonotonic: boolean | null
    /** Series count is glanceable on the drawer card. The detail
     *  pane lists timeseries directly below the header, so most
     *  callers there omit it. */
    seriesCount?: number
    seriesCardinality?: number
    dataPointCount?: number
  }

  type TraceProps = {
    signal: 'trace'
    spanCount: number
    errorCount: number
  }

  type LogProps = {
    signal: 'log'
    severityNumber: number
    severityText: string
  }

  export type SignalBadgesProps = MetricProps | TraceProps | LogProps
</script>

<script lang="ts">
  let props: SignalBadgesProps = $props()

  let metricTypeBadge = $derived.by(() => {
    if (props.signal !== 'metric') return null
    return metricTypeCardBadge(
      props.metricType,
      props.aggregationTemporality,
      props.isMonotonic
    )
  })
</script>

{#if props.signal === 'metric' && metricTypeBadge}
  <span class={metricTypeBadge.className} title={metricTypeBadge.title}>
    {metricTypeBadge.label}
  </span>
  {#if props.seriesCount !== undefined}
    {@const total = props.seriesCardinality}
    {@const partial = total !== undefined && total > props.seriesCount}
    {@const seriesTitle = partial
      ? `${props.seriesCount} of ${total} series reported in this range; the stream has ${total} in all`
      : `${props.seriesCount} time series in range`}
    <span
      class="badge-count"
      title={seriesTitle +
        (props.dataPointCount !== undefined
          ? `; ${props.dataPointCount} datapoints in range`
          : '')}
    >
      <span
        >{partial
          ? `${props.seriesCount} of ${total} series`
          : `${props.seriesCount} series`}</span
      >
      {#if props.dataPointCount !== undefined}
        <span class="opacity-40" aria-hidden="true">|</span>
        <span aria-label={`${props.dataPointCount} datapoints`}
          >{props.dataPointCount.toLocaleString()} pts</span
        >
      {/if}
    </span>
  {/if}
{:else if props.signal === 'trace'}
  <span
    class="badge-count"
    title="{props.spanCount} span{props.spanCount !== 1 ? 's' : ''}"
  >
    {props.spanCount} span{props.spanCount !== 1 ? 's' : ''}
  </span>
  {#if props.errorCount > 0}
    <span class="error-count">
      {props.errorCount} err
    </span>
  {/if}
{:else if props.signal === 'log'}
  {@const label = severityLabel(props.severityText, props.severityNumber)}
  <span
    class="{severityBadgeClass(props.severityNumber)} tabular-nums"
    title={label}
  >
    {label} ({props.severityNumber})
  </span>
{/if}
