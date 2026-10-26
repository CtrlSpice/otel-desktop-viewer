<script lang="ts">
  import { tick } from 'svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import SeriesDatapointList from './SeriesDatapointList.svelte'

  let { error, onretry }: { error?: string; onretry?: () => void } = $props()
  const ctx = getMetricViewContext()
  let panel: HTMLDivElement
  const series = $derived(
    ctx.metric?.timeseries.find(
      series => series.seriesRef === ctx.selectedSeriesKey
    )
  )
  const datapoints = $derived(
    series ? ctx.seriesDatapoints(series.seriesRef) : undefined
  )

  $effect(() => {
    const key = ctx.selectedSeriesKey
    if (!key) return
    let cancelled = false
    void tick().then(() => {
      if (cancelled) return
      const disclosure = panel.closest('details')
      if (disclosure) disclosure.open = true
    })
    return () => {
      cancelled = true
    }
  })
</script>

<div
  bind:this={panel}
  class="flex min-h-0 min-w-0 flex-1 flex-col"
  role="region"
  aria-label="Selected series datapoints"
>
  {#if series}
    {#if error}
      <p role="alert" class="px-3 py-2 text-sm text-papaya">{error}</p>
      {#if onretry}<button
          type="button"
          class="btn btn-ghost mx-3"
          onclick={onretry}>Retry datapoints</button
        >{/if}
    {:else if datapoints === undefined}
      <p role="status" class="px-3 py-2 text-sm text-subtle">
        Loading datapoints…
      </p>
    {:else}
      {#key series.seriesRef}
        <SeriesDatapointList {datapoints} />
      {/key}
    {/if}
  {:else}
    <p class="px-3 py-2 text-sm text-subtle">
      Select a series to inspect its datapoints.
    </p>
  {/if}
</div>
