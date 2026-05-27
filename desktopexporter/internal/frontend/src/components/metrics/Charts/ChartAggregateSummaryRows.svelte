<script lang="ts">
  import type { AggregateSummaryRow } from '@/components/metrics/utils/aggregation'
  import { AGG_COLOR_ALL, AGG_COLOR_SELECTED } from '@/utils/chart-palette'

  type Props = {
    rows: readonly AggregateSummaryRow[]
    class?: string
  }

  let { rows, class: className = '' }: Props = $props()

  function lineColor(variant: AggregateSummaryRow['variant']): string {
    return variant === 'secondary' ? AGG_COLOR_ALL : AGG_COLOR_SELECTED
  }
</script>

{#if rows.length > 0}
  <ul class="chart-aggregate-summary {className}">
    {#each rows as row (row.key)}
      <li class="chart-aggregate-summary__row">
        <span
          class="chart-aggregate-summary__line"
          style:--agg-color={lineColor(row.variant)}
          aria-hidden="true"
        ></span>
        <span class="chart-aggregate-summary__label">{row.label}:</span>
        <span class="chart-aggregate-summary__value">{row.valueText}</span>
      </li>
    {/each}
  </ul>
{/if}
