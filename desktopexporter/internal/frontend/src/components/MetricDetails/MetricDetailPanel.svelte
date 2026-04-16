<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity'
  import type { MetricData, DataPoint, HistogramDataPoint, ExponentialHistogramDataPoint } from '@/types/api-types'
  import { getServiceName } from '@/utils/resource'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { metricTypeBadgeClass } from '@/utils/metric-type'
  import MetricTimeSeriesChart from '@/components/MetricCharts/MetricTimeSeriesChart.svelte'
  import HistogramChart from '@/components/MetricCharts/HistogramChart.svelte'
  import { TrashIcon } from '@/icons'

  type Props = {
    metric: MetricData | undefined
    onDelete: (id: string) => void
  }

  let { metric, onDelete }: Props = $props()

  let timeContext = getTimeContext()

  let service = $derived(metric ? (getServiceName(metric.resource) ?? '') : '')

  let metricType = $derived(
    metric?.datapoints[0]?.metricType ?? 'Empty'
  )

  let latestHistogramDp = $derived.by(() => {
    if (!metric) return undefined
    if (metricType !== 'Histogram' && metricType !== 'ExponentialHistogram') return undefined
    const sorted = [...metric.datapoints]
      .filter(dp => dp.metricType === 'Histogram' || dp.metricType === 'ExponentialHistogram')
      .sort((a, b) => (a.timestamp > b.timestamp ? -1 : 1))
    return sorted[0] as HistogramDataPoint | ExponentialHistogramDataPoint | undefined
  })

  let expandedDatapoints = new SvelteSet<string>()

  function toggleDatapoint(id: string) {
    if (expandedDatapoints.has(id)) {
      expandedDatapoints.delete(id)
    } else {
      expandedDatapoints.add(id)
    }
  }

  function datapointValue(dp: DataPoint): string {
    if (dp.metricType === 'Gauge' || dp.metricType === 'Sum') {
      return String(dp.doubleValue ?? dp.intValue ?? '—')
    }
    if (dp.metricType === 'Histogram' || dp.metricType === 'ExponentialHistogram') {
      return `count: ${dp.count}, sum: ${dp.sum.toFixed(2)}`
    }
    return '—'
  }
</script>

{#if metric}
  <div class="metric-detail-panel">
    <div class="metric-detail-panel__scroll">
      <!-- Header -->
      <div class="metric-detail-header">
        <div class="flex items-center gap-2 min-w-0">
          <h2 class="text-base font-semibold truncate" title={metric.name}>{metric.name}</h2>
          <span class={metricTypeBadgeClass(metricType)}>{metricType}</span>
        </div>
        {#if metric.unit}
          <span class="text-xs text-base-content/50">Unit: {metric.unit}</span>
        {/if}
      </div>

      {#if metric.description}
        <p class="metric-detail-description">{metric.description}</p>
      {/if}

      <!-- Chart area -->
      <div class="metric-detail-chart">
        {#if metricType === 'Gauge' || metricType === 'Sum'}
          <MetricTimeSeriesChart datapoints={metric.datapoints} />
        {:else if latestHistogramDp}
          <HistogramChart datapoint={latestHistogramDp} />
        {:else}
          <div class="flex items-center justify-center h-[250px] text-base-content/40 text-sm">
            No chart available for this metric type
          </div>
        {/if}
      </div>

      <!-- Metadata table -->
      <table class="detail-fields w-full" aria-label="Metric details">
        <thead class="table-header-surface">
          <tr class="table-header-row">
            <th class="table-header-cell table-header-cell--left" colspan="3">
              Metadata
            </th>
          </tr>
        </thead>
        <tbody>
          <tr class="table-row">
            <td class="detail-cell" colspan="2"><span class="detail-cell__key">received:</span> <span class="tabular-nums">{formatTimestamp(metric.received, timeContext.timezone, 'milliseconds')}</span></td>
            <td class="detail-cell--badges"><span class="badge-type">timestamp</span></td>
          </tr>
          <tr class="table-row">
            <td class="detail-cell" colspan="2"><span class="detail-cell__key">service:</span> {service || '—'}</td>
            <td class="detail-cell--badges"><span class="badge-type">string</span></td>
          </tr>
          {#if metric.resourceDroppedAttributesCount > 0}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">dropped attributes:</span> {metric.resourceDroppedAttributesCount}</td>
              <td class="detail-cell--badges"><span class="badge-type">uint32</span></td>
            </tr>
          {/if}
          {#each metric.resource.attributes as attr (attr.key)}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">{attr.key}:</span> {attr.value}</td>
              <td class="detail-cell--badges"><span class="badge-type">{attr.type}</span> <span class="badge-origin">resource</span></td>
            </tr>
          {/each}
          {#if metric.scope.name}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">scope name:</span> {metric.scope.name}</td>
              <td class="detail-cell--badges"><span class="badge-type">string</span> <span class="badge-origin">scope</span></td>
            </tr>
          {/if}
          {#if metric.scope.version}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">scope version:</span> {metric.scope.version}</td>
              <td class="detail-cell--badges"><span class="badge-type">string</span> <span class="badge-origin">scope</span></td>
            </tr>
          {/if}
          {#each metric.scope.attributes as attr (attr.key)}
            <tr class="table-row">
              <td class="detail-cell" colspan="2"><span class="detail-cell__key">{attr.key}:</span> {attr.value}</td>
              <td class="detail-cell--badges"><span class="badge-type">{attr.type}</span> <span class="badge-origin">scope</span></td>
            </tr>
          {/each}
        </tbody>
      </table>

      <!-- Datapoints list -->
      <details class="metric-detail-datapoints" open>
        <summary class="table-header-surface metric-detail-datapoints__summary">
          Datapoints ({metric.datapoints.length})
        </summary>
        <div class="metric-detail-datapoints__list">
          {#each metric.datapoints as dp (dp.id)}
            {@const expanded = expandedDatapoints.has(dp.id)}
            <div class="metric-dp-row">
              <button
                type="button"
                class="metric-dp-row__header"
                onclick={() => toggleDatapoint(dp.id)}
              >
                <span class="tabular-nums text-base-content/70">{formatTimestamp(dp.timestamp, timeContext.timezone, 'milliseconds')}</span>
                <span class="text-base-content">{datapointValue(dp)}</span>
                {#if dp.attributes.length > 0}
                  <span class="badge badge-xs badge-soft badge-neutral">{dp.attributes.length} attr{dp.attributes.length !== 1 ? 's' : ''}</span>
                {/if}
                <svg class="w-3 h-3 shrink-0 transition-transform {expanded ? 'rotate-180' : ''}" viewBox="0 0 24 24">
                  <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
                </svg>
              </button>
              {#if expanded}
                <div class="metric-dp-row__details">
                  {#each dp.attributes as attr (attr.key)}
                    <div class="metric-dp-attr">
                      <span class="detail-cell__key">{attr.key}:</span>
                      <span>{attr.value}</span>
                      <span class="badge-type ml-auto">{attr.type}</span>
                    </div>
                  {/each}
                  {#if dp.exemplars.length > 0}
                    <div class="metric-dp-exemplars-header">Exemplars</div>
                    {#each dp.exemplars as ex (ex.timestamp)}
                      <div class="metric-dp-attr">
                        <span class="tabular-nums text-base-content/70">{formatTimestamp(ex.timestamp, timeContext.timezone, 'milliseconds')}</span>
                        <span>value: {ex.value}</span>
                        {#if ex.traceID}
                          <a href="/trace/{ex.traceID}" class="link link-primary text-xs font-mono ml-auto">trace</a>
                        {/if}
                      </div>
                    {/each}
                  {/if}
                </div>
              {/if}
            </div>
          {/each}
        </div>
      </details>
    </div>

    <!-- Footer -->
    <div class="metric-detail-panel__footer">
      <button
        type="button"
        class="btn btn-ghost btn-sm text-error"
        onclick={() => onDelete(metric.id)}
        aria-label="Delete this metric"
      >
        <TrashIcon class="h-3.5 w-3.5" aria-hidden="true" />
        Delete this metric
      </button>
    </div>
  </div>
{:else}
  <div class="metric-detail-panel metric-detail-panel--empty">
    <p class="text-base-content/40 text-sm">Select a metric to view details</p>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .metric-detail-panel {
    @apply flex h-full flex-col overflow-hidden;
  }

  .metric-detail-panel--empty {
    @apply items-center justify-center;
  }

  .metric-detail-panel__scroll {
    @apply flex-1 min-h-0 overflow-y-auto;
  }

  .metric-detail-header {
    @apply flex flex-col gap-1 px-4 py-3 border-b border-base-300/30;
  }

  .metric-detail-description {
    @apply px-4 py-2 text-sm text-base-content/70 border-b border-base-300/30;
  }

  .metric-detail-chart {
    @apply px-2 py-3 border-b border-base-300/30;
  }

  .metric-detail-datapoints {
    @apply w-full;
  }

  .metric-detail-datapoints__summary {
    @apply cursor-pointer select-none px-4 text-xs font-semibold tracking-normal text-base-content/55;
  }

  .metric-detail-datapoints__list {
    @apply max-h-64 overflow-y-auto;
  }

  .metric-dp-row {
    @apply border-b border-base-300/20;
  }

  .metric-dp-row__header {
    @apply flex w-full items-center gap-3 px-4 py-1.5 text-xs text-left hover:bg-base-200/50 transition-colors;
  }

  .metric-dp-row__details {
    @apply px-6 pb-2 space-y-0.5;
  }

  .metric-dp-attr {
    @apply flex items-center gap-2 text-xs py-0.5;
  }

  .metric-dp-exemplars-header {
    @apply text-xs font-semibold text-base-content/55 pt-1;
  }

  .metric-detail-panel__footer {
    @apply flex items-center justify-end gap-2 border-t border-base-300/50 px-4 py-2;
  }
</style>
