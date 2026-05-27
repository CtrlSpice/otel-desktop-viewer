<script lang="ts">
  /*
   * Inline datapoint rows for a single timeseries. Nested under an
   * expanded series row in TimeseriesPanel. Selection, exemplar
   * expansion, and histogram snapshot sync go through MetricViewContext.
   */
  import { formatDateTimeMs } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { formatMetricValuePlain } from '@/components/metrics/utils/format-metric-value'
  import type { DataPoint } from '@/types/api-types'

  type Props = {
    datapoints: DataPoint[]
    /** Show a color swatch column (flat cross-series lists). */
    showSwatch?: boolean
    seriesColor?: string
    /** Drop horizontal inset (nested under series rows). */
    flush?: boolean
  }

  let { datapoints, showSwatch = false, seriesColor, flush = false }: Props =
    $props()

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  let metricUnit = $derived(ctx.metric?.unit ?? '')

  function displayUnit(unit: string): string | null {
    const u = unit.trim()
    if (!u || u === '1') return null
    return u
  }

  function formatDatapointTime(timestamp: bigint): string {
    return formatDateTimeMs(
      Number(timestamp / 1_000_000n),
      timeContext.timezone
    ).dateTime
  }

  function datapointValueParts(
    dp: DataPoint
  ): { number: string; unit: string | null } {
    const unit = displayUnit(metricUnit)
    if (dp.metricType === 'Gauge' || dp.metricType === 'Sum') {
      const raw = dp.doubleValue ?? dp.intValue
      if (raw === undefined || raw === null) {
        return { number: '—', unit: null }
      }
      return {
        number: formatMetricValuePlain(Number(raw)),
        unit,
      }
    }
    if (
      dp.metricType === 'Histogram' ||
      dp.metricType === 'ExponentialHistogram'
    ) {
      return {
        number: `count ${dp.count}, sum ${formatMetricValuePlain(dp.sum)}`,
        unit,
      }
    }
    return { number: '—', unit: null }
  }
</script>

{#if datapoints.length === 0}
  <p class="dp-list__empty" class:dp-list__empty--flush={flush}>No datapoints</p>
{:else}
  <table class="dp-list" class:dp-list--flush={flush} aria-label="Datapoints">
    <tbody>
      {#each datapoints as dp (dp.id)}
        {@const selected = ctx.selectedDatapointId === dp.id}
        {@const hasExtra = dp.flags > 0 || dp.exemplars.length > 0}
        {@const expanded = hasExtra && ctx.expandedDatapoints.has(dp.id)}
        {@const valueParts = datapointValueParts(dp)}
        <tr
          class="dp-list__row"
          class:dp-list__row--selected={selected}
          class:dp-list__row--expandable={hasExtra}
          data-dp-id={dp.id}
          onclick={() => ctx.onDatapointClick(dp)}
        >
          {#if showSwatch}
            <td class="dp-list__td dp-list__td--swatch">
              <span
                class="dp-list__swatch"
                style:background-color={seriesColor}
                aria-hidden="true"
              ></span>
            </td>
          {/if}
          <td
            class="dp-list__td dp-list__td--content"
            colspan={showSwatch ? undefined : 1}
          >
            <div class="dp-list__row-main">
              <span class="dp-list__time tabular-nums"
                >{formatDatapointTime(dp.timestamp)}</span
              >
              <div class="dp-list__trail">
                <span class="dp-list__value-group">
                  <span class="dp-list__value tabular-nums">{valueParts.number}</span>
                  {#if valueParts.unit}
                    <span class="dp-list__unit">{valueParts.unit}</span>
                  {/if}
                </span>
                {#if hasExtra}
                  {#if dp.exemplars.length > 0}
                    <span class="badge-count">{dp.exemplars.length} ex</span>
                  {/if}
                  {#if dp.flags > 0}
                    <span class="badge badge-xs badge-soft badge-warning">flags</span>
                  {/if}
                {/if}
              </div>
            </div>
          </td>
        </tr>
        {#if expanded}
          <tr class="dp-list__expansion-row">
            <td colspan={showSwatch ? 2 : 1} class="dp-list__expansion-cell">
              <div class="dp-list__expansion">
                {#if dp.flags > 0}
                  <div class="dp-list__detail">
                    <span class="dp-list__detail-label">flags</span>
                    <span class="dp-list__detail-value">{dp.flags}</span>
                  </div>
                {/if}
                {#each dp.exemplars as ex, i}
                  <div class="dp-list__exemplar">
                    <span class="dp-list__detail-label">exemplar {i + 1}</span>
                    <div class="dp-list__exemplar-fields">
                      <span class="dp-list__detail-value">value: {ex.value}</span>
                      <span class="dp-list__detail-value tabular-nums">
                        time: {formatDatapointTime(ex.timestamp)}
                      </span>
                      {#if ex.traceID}
                        <span class="dp-list__detail-value">trace: {ex.traceID}</span>
                      {/if}
                      {#if ex.spanID}
                        <span class="dp-list__detail-value">span: {ex.spanID}</span>
                      {/if}
                      {#each ex.filteredAttributes as attr (attr.key)}
                        <span class="dp-list__detail-value">{attr.key}: {attr.value}</span>
                      {/each}
                    </div>
                  </div>
                {/each}
              </div>
            </td>
          </tr>
        {/if}
      {/each}
    </tbody>
  </table>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .dp-list__empty--flush {
    @apply px-0;
  }

  .dp-list--flush .dp-list__td--swatch {
    @apply pl-0;
  }

  .dp-list--flush .dp-list__td--content {
    @apply pl-0 pr-2;
  }

  .dp-list__empty {
    @apply m-0 px-3 py-2 text-center text-xs italic;
    color: var(--color-muted);
  }

  .dp-list {
    @apply w-full text-xs;
    border-collapse: collapse;
  }

  .dp-list__row {
    @apply cursor-pointer transition-colors hover:bg-base-300/30;
  }

  .dp-list__row--selected {
    background-color: color-mix(
      in oklab,
      var(--color-primary) 18%,
      transparent
    );
  }

  .dp-list__td {
    @apply py-1 align-middle;
  }

  .dp-list__td--swatch {
    @apply pl-3 pr-1;
    width: 1.25rem;
  }

  .dp-list__swatch {
    @apply inline-block rounded-full;
    width: 6px;
    height: 6px;
  }

  .dp-list__td--content {
    @apply w-full pl-1 pr-4;
  }

  .dp-list__row-main {
    @apply flex w-full min-w-0 items-baseline justify-between gap-3;
  }

  .dp-list__trail {
    @apply flex min-w-0 shrink items-baseline justify-end gap-2;
  }

  .dp-list__value-group {
    @apply inline-flex min-w-0 items-baseline justify-end gap-1;
  }

  .dp-list__time {
    @apply shrink-0;
    color: var(--color-subtle);
  }

  .dp-list__value {
    @apply min-w-0 truncate font-mono;
    color: var(--color-base-content);
  }

  .dp-list__unit {
    @apply shrink-0;
    color: var(--color-subtle);
  }

  .dp-list__expansion-row {
    @apply bg-base-200/50;
  }

  .dp-list__expansion-cell {
    @apply p-0;
  }

  .dp-list__expansion {
    @apply flex flex-col gap-2 px-4 py-2;
    border-bottom: 1px solid
      color-mix(in oklab, var(--color-base-300) 30%, transparent);
  }

  .dp-list__detail {
    @apply flex items-baseline gap-2 text-xs;
  }

  .dp-list__detail-label {
    @apply shrink-0 text-xs font-medium;
    color: var(--color-subtle);
  }

  .dp-list__detail-value {
    @apply font-mono text-xs;
    color: var(--color-base-content);
  }

  .dp-list__exemplar {
    @apply flex flex-col gap-0.5;
  }

  .dp-list__exemplar-fields {
    @apply flex flex-col gap-0.5 pl-3;
  }
</style>
