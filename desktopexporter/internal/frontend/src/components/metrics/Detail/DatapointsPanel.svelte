<script lang="ts">
  import { tick, type Snippet } from 'svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import SeriesDatapointList, {
    type DatapointSeries,
  } from './SeriesDatapointList.svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'

  let {
    response,
    icon,
    error,
    onretry,
    selectionCurrent = true,
  }: {
    response?: DatapointSeries
    icon?: Snippet
    error?: string
    onretry?: () => void
    selectionCurrent?: boolean
  } = $props()
  const ctx = getMetricViewContext()
  let panel: HTMLDivElement | undefined = $state()
  const series = $derived(
    (ctx.selectionSource === 'chart'
      ? ctx.chartMetric
      : ctx.metric
    )?.timeseries.find(series => series.seriesRef === ctx.selectedSeriesKey)
  )
  let displayed = $state.raw<DatapointSeries>()
  // This component owns availability and stays mounted when its body is empty.
  // Rows, counts and identifying metadata share the received-series response.
  $effect.pre(() => {
    if (!series) displayed = undefined
    else if (response) displayed = response
    else if (displayed?.count === 0) {
      // An earlier empty result cannot establish absence for a new request.
      displayed = undefined
    }
  })
  $effect(() => {
    const key = ctx.selectedSeriesKey
    if (!key) return
    let cancelled = false
    void tick().then(() => {
      if (cancelled) return
      const disclosure = panel?.closest('details')
      if (disclosure) disclosure.open = true
    })
    return () => {
      cancelled = true
    }
  })
</script>

<DetailBubble
  label="Datapoints"
  contained
  {icon}
  disabled={!series}
  count={displayed?.count}
>
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
      {/if}
      {#if !response && !error}
        <LoadingStatus
          pending
          label="Loading datapoints…"
          requestKey={series.seriesRef}
          class="block px-3 py-2 text-sm text-subtle"
        />
      {/if}
      {#if displayed}
        <div
          class="px-3 py-2 text-subtle"
          aria-label="Displayed datapoint series"
        >
          Metric <Identifier value={displayed.metricRef} /> · Series <Identifier
            value={displayed.seriesRef}
          />
        </div>
        {#key JSON.stringify([displayed.metricRef, displayed.seriesRef])}
          <SeriesDatapointList
            {selectionCurrent}
            datapoints={displayed.datapoints}
            owner={displayed}
          />
        {/key}
      {/if}
    {:else}
      <p class="px-3 py-2 text-sm text-subtle">
        Select a series to inspect its datapoints.
      </p>
    {/if}
  </div>
</DetailBubble>
