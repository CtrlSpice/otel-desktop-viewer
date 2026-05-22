<script lang="ts" module>
  /** A row in the mini-legend. Generic on purpose: histogram/gauge
   *  futures will fill this in their own way (e.g. quantile rows,
   *  bucket-count rows). The component itself doesn't know about
   *  metric kinds. */
  export type SelectionLegendRow = {
    /** Stable identity for keyed-each. */
    key: string
    /** Hex / token resolved by the caller from its color-by-key map. */
    color: string
    /** Display text (series label, aggregate label, etc.). */
    label: string
    /** Optional 1-2 char glyph to the left of the label (we use it for
     *  aggregation operator symbols: Σ / μ / Δ/t). */
    glyph?: string | null
    /** Accessible title for the glyph (read by screen readers when set). */
    glyphTitle?: string | null
    /** Pre-formatted value string. Caller formats so this component
     *  doesn't have to care about units or decimal precision. */
    valueText: string
    /** Marks the user-picked row. Bolder weight + a colored left
     *  accent so the eye anchors on it even amongst aggregates. */
    isPrimary?: boolean
  }
</script>

<script lang="ts">
  /*
   * ChartSelectionLegend: a small floating card showing the timestamp
   * of a clicked datapoint plus per-series values at that x. Sits in
   * the corner of a chart (caller positions it via absolute layout).
   *
   * Design choices baked in:
   * - Caller owns positioning. Component is just the card; the chart
   *   wraps it in an absolute-positioned host.
   * - pointer-events: none so it never steals hover/click from chart.
   * - Rows are pre-built by the caller. Component does no lookups, no
   *   formatting, no key-matching. Keeps it reusable across chart
   *   types that pick rows very differently.
   */
  type Props = {
    /** Pre-formatted timestamp string (e.g. "16:42:18.123 PDT").
     *  Caller decides resolution and timezone so we match whatever
     *  other timestamps in the same view are using. */
    timestamp: string
    /** Rows to show, in render order. Empty array hides the whole
     *  card (caller can also conditionally render the component, but
     *  this lets the component own its own emptiness gracefully). */
    rows: readonly SelectionLegendRow[]
  }

  let { timestamp, rows }: Props = $props()
</script>

{#if rows.length > 0}
  <div class="chart-selection-legend" aria-live="polite">
    <div class="chart-selection-legend__timestamp">{timestamp}</div>
    <ul class="chart-selection-legend__rows">
      {#each rows as row (row.key)}
        <li
          class="chart-selection-legend__row"
          class:chart-selection-legend__row--primary={row.isPrimary}
        >
          <span
            class="chart-selection-legend__dot"
            style:--color={row.color}
            aria-hidden="true"
          ></span>
          <span class="chart-selection-legend__label">
            {#if row.glyph}<span
                class="chart-selection-legend__glyph"
                title={row.glyphTitle ?? undefined}
                aria-label={row.glyphTitle ?? undefined}
              >{row.glyph}</span>
            {/if}{row.label}
          </span>
          <span class="chart-selection-legend__value">{row.valueText}</span>
        </li>
      {/each}
    </ul>
  </div>
{/if}

<!-- Styles live in app.css under .metric-chart-view :where(.chart-selection-legend) -->
