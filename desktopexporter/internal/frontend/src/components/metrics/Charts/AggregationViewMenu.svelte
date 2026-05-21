<script lang="ts">
  /*
   * AggregationViewMenu: the view-picker dropdown that lives in the
   * chart PaneHeader's right slot for Sum and Gauge metrics. Mirrors
   * the sort popover pattern in DrawerSearchPanel.svelte (anchored
   * popover + menuitem radio buttons + active-state class).
   *
   * Reads `aggregationView` and writes via `setAggregationView` on
   * MetricViewContext. The parent decides whether to render this
   * component at all (current rule: metric type === 'Sum' || 'Gauge');
   * the menu itself does not gate on metric type.
   */
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import {
    createPopoverId,
    setupAnchorPopover,
  } from '@/components/shared/utils/anchor-popover'
  import { ArrowDownIcon } from '@/icons'
  import type { AggregationView } from '@/components/metrics/utils/aggregation'

  // Static option list. Mapping kept in-component because labels are
  // UI text and don't belong in the pure aggregation module. The order
  // here is the menu order; the visible subset is filtered against
  // ctx.availableAggregationViews so we only ever offer options that
  // change the chart in a meaningful way.
  const VIEW_OPTIONS: ReadonlyArray<{ value: AggregationView; label: string }> = [
    { value: 'raw', label: 'Raw' },
    { value: 'sum', label: 'Sum' },
    { value: 'avg', label: 'Average' },
    { value: 'rate', label: 'Rate' },
  ]

  const ctx = getMetricViewContext()

  // Filtered against availability rules: Sum/Avg require >=2 series;
  // Rate requires cumulative-monotonic shape. See
  // availableAggregationViews() in aggregation.ts.
  let visibleOptions = $derived(
    VIEW_OPTIONS.filter(o => ctx.availableAggregationViews.includes(o.value))
  )

  // When only one option is available (commonly N=1 non-cumulative-
  // monotonic), the menu becomes a disabled label rather than an
  // interactive control.
  let menuDisabled = $derived(visibleOptions.length <= 1)

  let triggerEl = $state<HTMLButtonElement | null>(null)
  let popoverEl = $state<HTMLDivElement | null>(null)
  let popoverOpen = $state(false)

  const popoverId = createPopoverId('aggregation-view-popover')

  // Current view's display label. Falls back to capitalized id if the
  // option list ever drifts from the AggregationView union.
  let currentLabel = $derived(
    VIEW_OPTIONS.find(o => o.value === ctx.aggregationView)?.label ??
      ctx.aggregationView.charAt(0).toUpperCase() + ctx.aggregationView.slice(1)
  )

  $effect(() => {
    const trigger = triggerEl
    const popover = popoverEl
    if (!trigger || !popover) return
    return setupAnchorPopover({
      popover,
      trigger,
      anchor: 'below-end',
      onOpenChange: open => {
        popoverOpen = open
      },
    })
  })

  function selectView(view: AggregationView) {
    ctx.setAggregationView(view)
    popoverEl?.hidePopover()
  }
</script>

<button
  bind:this={triggerEl}
  type="button"
  class="aggregation-view-menu__trigger btn btn-sm shrink-0"
  popovertarget={menuDisabled ? undefined : popoverId}
  aria-expanded={popoverOpen}
  aria-label="Chart view"
  aria-disabled={menuDisabled}
  disabled={menuDisabled}
>
  <span class="aggregation-view-menu__trigger-value">{currentLabel}</span>
  {#if !menuDisabled}
    <ArrowDownIcon class="h-3 w-3 shrink-0 opacity-70" aria-hidden="true" />
  {/if}
</button>

<div
  bind:this={popoverEl}
  popover="auto"
  id={popoverId}
  class="anchor-popover anchor-popover--anchored anchor-popover--menu"
>
  <ul class="anchor-popover-menu" role="menu" aria-label="Chart view">
    {#each visibleOptions as opt (opt.value)}
      <li role="none">
        <button
          type="button"
          role="menuitemradio"
          aria-checked={opt.value === ctx.aggregationView}
          class="anchor-popover-menu__option {opt.value === ctx.aggregationView
            ? 'anchor-popover-menu__option--active'
            : ''}"
          onclick={() => selectView(opt.value)}
        >
          <span>{opt.label}</span>
        </button>
      </li>
    {/each}
  </ul>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  /* Width is pinned so the trigger doesn't jump as the user switches
     between 'Raw' (3ch) and 'Average' (7ch). 6.5rem fits 'Average ▾'
     comfortably with the px-3 horizontal padding. */
  .aggregation-view-menu__trigger {
    @apply gap-1.5 px-3 font-semibold bg-primary/10 text-primary
           rounded-full border-none shadow-none;
    min-width: 6.5rem;
  }

  .aggregation-view-menu__trigger:hover:not(:disabled) {
    @apply bg-primary/15;
  }

  .aggregation-view-menu__trigger:disabled {
    @apply cursor-default opacity-70;
  }

  .aggregation-view-menu__trigger-value {
    @apply flex-1 text-center text-xs font-medium;
  }
</style>
