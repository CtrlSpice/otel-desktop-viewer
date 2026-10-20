<script module lang="ts">
  /* Shared drawer-card and detail-header badges from primitive signal facts. */
  import type { MetricType } from '@/types/api-types'
  import { metricTypeSummary } from '@/components/metrics/utils/metric-type'
  import LogSeverity from '@/components/logs/LogSeverity.svelte'

  type MetricProps = {
    signal: 'metric'
    metricType: MetricType | string
    aggregationTemporality: string | null | undefined
    isMonotonic: boolean | null
    showType?: boolean
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

  let metricType = $derived.by(() => {
    if (props.signal !== 'metric') return null
    return metricTypeSummary(
      props.metricType,
      props.aggregationTemporality,
      props.isMonotonic
    )
  })
</script>

{#if props.signal === 'metric' && metricType}
  {#if props.showType !== false}
    <span class="text-sm text-secondary" title={metricType.title}>
      {metricType.label}
    </span>
  {/if}
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
        <span aria-hidden="true">·</span>
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
    {#if props.errorCount > 0}
      <span class="inline-flex items-center gap-2">
        <span class="size-2 shrink-0 rounded-full bg-error" aria-hidden="true"
        ></span>
        {props.errorCount} err</span
      >{' · '}
    {/if}
    {props.spanCount} span{props.spanCount !== 1 ? 's' : ''}
  </span>
{:else if props.signal === 'log'}
  <LogSeverity number={props.severityNumber} text={props.severityText} />
{/if}
