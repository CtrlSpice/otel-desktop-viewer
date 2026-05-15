<script lang="ts" module>
  import type { Attribute } from '@/types/api-types'

  /**
   * One per-attribute timeseries in the chart. The caller is expected
   * to have already grouped its datapoints into these timeseries
   * entries and to pass them in the same order as the chart renders
   * them, so the n-th legend row's swatch colour matches the n-th
   * line on the chart (both come from `timeseriesColor(index)`).
   *
   * Lives in <script module> so callers can `import { type Timeseries }`
   * from this .svelte file -- instance-script exports aren't visible
   * to TS consumers.
   */
  export type Timeseries = {
    /** Stable identifier for this attribute set, used as the bind key.
     * In practice this is the `attributesKey` (canonical "key=value|..."
     * string) the backend materialises on every datapoint and bucket-
     * series point as `attrs_canonical`. The same encoding is used for
     * Gauge/Sum, Histogram, and ExponentialHistogram timeseries, so a
     * single legend implementation covers all metric types. */
    key: string
    /** Attributes that distinguish this timeseries from siblings. May
     * be empty for a metric whose datapoints carry no attributes. */
    attributes: Attribute[]
    /** Optional sample count or other small annotation shown after the
     * attribute pairs. Purely informational; not bound. */
    badge?: string
  }
</script>

<script lang="ts">
  import type { SvelteSet } from 'svelte/reactivity'
  import {
    MAX_VISIBLE_TIMESERIES,
    timeseriesColor,
    timeseriesForegroundColor,
  } from '@/utils/timeseries-palette'

  type Props = {
    timeseries: Timeseries[]
    /** SvelteSet of timeseries keys currently visible on the chart.
     * The legend mutates the set in place via .add() / .delete();
     * SvelteSet's reactivity propagates writes to every reader
     * (chart, datapoints filter, etc.) without us having to copy. */
    visibleKeys: SvelteSet<string>
  }

  let { timeseries, visibleKeys }: Props = $props()

  // Visible-set cap (max). Hitting MAX_VISIBLE_TIMESERIES disables
  // every *unchecked* row so the palette never needs to wrap; checked
  // rows stay enabled so the user can always uncheck and free a slot.
  let capReached = $derived(visibleKeys.size >= MAX_VISIBLE_TIMESERIES)

  // Visible-set floor (min). When exactly one row is checked, that
  // row's checkbox is locked so the chart can't go fully blank. Pairs
  // with capReached above; together they enforce 1 <= visible <= 10.
  // The min-1 rule is communicated per-row via a tooltip rather than
  // a global banner because there's only ever one row affected at a
  // time -- a banner would be louder than the situation warrants.
  let isLastChecked = $derived(visibleKeys.size === 1)

  function toggle(key: string, checked: boolean) {
    if (checked) {
      visibleKeys.add(key)
    } else {
      visibleKeys.delete(key)
    }
  }
</script>

<div
  class="timeseries-legend"
  role="group"
  aria-label="Timeseries visibility"
>
  <div class="timeseries-legend__header">
    <span class="timeseries-legend__title">Timeseries</span>
    <span
      class="timeseries-legend__count"
      class:timeseries-legend__count--cap={capReached}
    >
      {visibleKeys.size} / {MAX_VISIBLE_TIMESERIES}
    </span>
  </div>

  <ul class="timeseries-legend__list">
    {#each timeseries as ts, i (ts.key)}
      {@const checked = visibleKeys.has(ts.key)}
      {@const disabledByCap = !checked && capReached}
      {@const disabledByFloor = checked && isLastChecked}
      {@const disabled = disabledByCap || disabledByFloor}
      {@const color = timeseriesColor(i)}
      {@const fg = timeseriesForegroundColor(i)}
      {@const tooltip = disabledByFloor
        ? 'At least one timeseries must remain selected'
        : ts.attributes.length === 0
          ? 'default'
          : ts.attributes.map((a) => `${a.key}=${a.value}`).join(', ')}
      <li
        class="timeseries-legend__row"
        class:timeseries-legend__row--disabled={disabled}
      >
        <label class="timeseries-legend__label" title={tooltip}>
          <input
            type="checkbox"
            class="checkbox checkbox-xs timeseries-legend__checkbox"
            style:--input-color={color}
            style:color={fg}
            {checked}
            {disabled}
            onchange={(e) =>
              toggle(ts.key, (e.currentTarget as HTMLInputElement).checked)}
          />
          <span class="timeseries-legend__attrs">
            {#if ts.attributes.length === 0}
              <span class="timeseries-legend__attrs-empty">default</span>
            {:else}
              {#each ts.attributes as attr (attr.key)}
                <span class="timeseries-legend__attr">
                  <span class="timeseries-legend__attr-key">{attr.key}</span>
                  <span class="timeseries-legend__attr-eq">=</span>
                  <span class="timeseries-legend__attr-value">{attr.value}</span>
                </span>
              {/each}
            {/if}
          </span>
          {#if ts.badge}
            <span class="timeseries-legend__badge">{ts.badge}</span>
          {/if}
        </label>
      </li>
    {/each}
  </ul>

  {#if capReached}
    <p class="timeseries-legend__cap-note">
      Cap of {MAX_VISIBLE_TIMESERIES} timeseries reached. Uncheck one to enable
      another.
    </p>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .timeseries-legend {
    @apply flex flex-col gap-2;
  }

  .timeseries-legend__header {
    @apply flex items-baseline justify-between;
  }

  .timeseries-legend__title {
    @apply text-xs font-semibold uppercase tracking-wide text-base-content/60;
  }

  .timeseries-legend__count {
    @apply text-xs tabular-nums text-base-content/60;
  }

  .timeseries-legend__count--cap {
    @apply text-warning font-semibold;
  }

  .timeseries-legend__list {
    @apply flex flex-col gap-1 m-0 p-0 list-none;
  }

  .timeseries-legend__row {
    @apply rounded;
  }

  .timeseries-legend__row--disabled {
    @apply opacity-50;
  }

  .timeseries-legend__label {
    @apply flex items-center gap-2 px-2 py-1 rounded cursor-pointer hover:bg-base-200/60;
    min-height: 1.75rem;
  }

  .timeseries-legend__row--disabled .timeseries-legend__label {
    @apply cursor-not-allowed hover:bg-transparent;
  }

  /* daisyUI .checkbox handles the box, the size (.checkbox-xs), and
     the checked-state colour (via the inline `--input-color` per row).
     We just keep the row-flex contract: shrink-0 so a long attrs list
     never crushes the box, and pointer cursor parity with the label. */
  .timeseries-legend__checkbox {
    @apply shrink-0 cursor-pointer;
  }

  .timeseries-legend__row--disabled .timeseries-legend__checkbox {
    @apply cursor-not-allowed;
  }

  /* Sits in a narrow column alongside the chart, so attribute pairs
     get one line each and overflow truncates with an ellipsis. The
     <label> wrapper carries `title` to surface the full text on
     hover -- without that an attribute like
     `service.namespace=production-us-east-1` becomes
     `service.namespace=produ…` and there's no way to recover it. */
  .timeseries-legend__attrs {
    @apply flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-xs font-mono min-w-0 overflow-hidden;
  }

  .timeseries-legend__attrs-empty {
    @apply italic text-base-content/50;
  }

  .timeseries-legend__attr {
    @apply inline-flex items-baseline min-w-0 max-w-full;
  }

  .timeseries-legend__attr-key {
    @apply text-base-content/70 shrink-0;
  }

  .timeseries-legend__attr-eq {
    @apply text-base-content/40 mx-0.5 shrink-0;
  }

  .timeseries-legend__attr-value {
    @apply text-base-content truncate;
  }

  .timeseries-legend__badge {
    @apply ml-auto text-[0.65rem] tabular-nums text-base-content/50 shrink-0;
  }

  .timeseries-legend__cap-note {
    @apply text-xs text-warning/80 m-0;
  }
</style>
