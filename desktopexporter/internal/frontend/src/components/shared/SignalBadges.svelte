<script module lang="ts">
  /*
   * SignalBadges: the single source of truth for the badge cluster
   * we render on each signal type's drawer card AND its detail
   * pane header. If the trace card adds a "warn" badge tomorrow,
   * the trace pane header gets it for free.
   *
   * The component is discriminated by `signal` and takes only the
   * primitive facts it needs to render — not the full summary or
   * detail data type. Callers translate from whatever they have on
   * hand (MetricSummary, MetricData + view ctx, SpanData[], …) into
   * the small shape this component expects. Keeps the component
   * decoupled from data sources and easy to use anywhere.
   */
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
    {#if total !== undefined && total > props.seriesCount}
      <!-- Both, because they answer different questions and the difference is
           the interesting part: series go quiet, and a count that silently
           drops looks like data loss rather than a narrower window. Shown as
           one badge rather than two so the relationship is legible at a
           glance. -->
      <span
        class="badge-count"
        title="{props.seriesCount} of {total} series reported in this range; the stream has {total} in all"
      >
        {props.seriesCount} of {total} series
      </span>
    {:else}
      <!-- Equal, which is the unbounded-window case: one number, because
           "12 of 12" is noise. -->
      <span
        class="badge-count"
        title="{props.seriesCount} time series in range"
      >
        {props.seriesCount} series
      </span>
    {/if}
  {/if}
{:else if props.signal === 'trace'}
  <span
    class="badge-count"
    title="{props.spanCount} span{props.spanCount !== 1 ? 's' : ''}"
  >
    {props.spanCount} span{props.spanCount !== 1 ? 's' : ''}
  </span>
  {#if props.errorCount > 0}
    <span class="badge badge-xs badge-soft badge-error tabular-nums">
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
