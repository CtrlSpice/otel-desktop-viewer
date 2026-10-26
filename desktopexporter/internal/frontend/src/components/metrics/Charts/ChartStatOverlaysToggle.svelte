<script lang="ts">
  import type { MetricViewContext } from '@/contexts/metric-view-context.svelte'
  import FilterPopover from '@/components/shared/Toolbar/FilterPopover.svelte'

  let { ctx }: { ctx: MetricViewContext } = $props()
  const overlayLabel = $derived(
    ctx.isHistogramKind ? 'Percentiles' : 'Statistics'
  )
  const combinedViews = $derived(
    ctx.availableAggregationViews.filter(
      view => view === 'sum' || view === 'avg'
    )
  )
</script>

{#snippet annotations()}
  <label
    class="btn btn-xs chart-header-control chart-stat-overlays-toggle tooltip tooltip-top"
    data-tip={ctx.isHistogramKind
      ? 'p50, p95 and p99 of the combined checked series'
      : ctx.rateSlopeOverlayAvailable
        ? 'Min/max and selected rate slope'
        : 'Min/max and average'}
  >
    <input
      type="checkbox"
      class="checkbox checkbox-xs chart-stat-overlays-toggle__checkbox"
      checked={ctx.showSelectionStatOverlays}
      aria-label="Show {overlayLabel}"
      onchange={e => ctx.setShowSelectionStatOverlays(e.currentTarget.checked)}
    />
    <span class="chart-stat-overlays-toggle__label"
      >{ctx.isHistogramKind
        ? overlayLabel
        : ctx.rateSlopeOverlayAvailable
          ? 'Min / max / slope'
          : 'Min / max / average'}</span
    >
  </label>
{/snippet}

{#if ctx.showChartStatOverlaysToggleVisible}
  {#if ctx.isHistogramKind}
    {@render annotations()}
  {:else}
    <FilterPopover
      label="Statistics"
      compact
      role="dialog"
      showHeader={false}
      triggerClass="btn btn-xs chart-header-control tooltip tooltip-top"
    >
      {#snippet icon()}<span>Statistics</span>{/snippet}
      <div class="flex flex-col gap-3">
        {#if ctx.aggregationView !== 'rate' && combinedViews.length > 0}
          <label class="flex items-center justify-between gap-3 text-subtle">
            Combined line
            <select
              class="select select-sm text-base-content"
              aria-label="Combined line"
              value={ctx.aggregationView}
              onchange={event => {
                const value = event.currentTarget.value
                if (value === 'raw' || value === 'sum' || value === 'avg')
                  ctx.setAggregationView(value)
              }}
            >
              <option value="raw">Off</option>
              {#each combinedViews as view (view)}
                <option value={view}
                  >{view === 'sum' ? 'Sum' : 'Average'}</option
                >
              {/each}
            </select>
          </label>
        {/if}
        {@render annotations()}
      </div>
    </FilterPopover>
  {/if}
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .chart-stat-overlays-toggle {
    @apply relative shrink-0 gap-2;
  }

  .chart-stat-overlays-toggle__label {
    @apply whitespace-nowrap;
  }
</style>
