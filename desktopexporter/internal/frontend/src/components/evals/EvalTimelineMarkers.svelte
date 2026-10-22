<script lang="ts">
  import TimelineMarkers from '@/components/traces/Waterfall/TimelineMarkers.svelte'
  import {
    clusterTimelineRecords,
    MARKER_CLUSTER_DISTANCE_REM,
    type TimelineRecord,
  } from '@/components/traces/Waterfall/timeline-markers'
  import { remToPx } from '@/state/panel-width'

  let {
    records,
    start,
    end,
    spanStart,
    color,
    onSelect,
  }: {
    records: TimelineRecord[]
    start: bigint
    end: bigint
    spanStart: bigint
    color: string
    onSelect: (record: TimelineRecord) => void
  } = $props()
  let width = $state(0)
  const markers = $derived(
    clusterTimelineRecords(
      records,
      start,
      end,
      width,
      remToPx(MARKER_CLUSTER_DISTANCE_REM)
    )
  )
</script>

<div class="eval-timeline-markers" bind:clientWidth={width}>
  <TimelineMarkers
    {markers}
    spanColor={color}
    spanStartTime={spanStart}
    onSelectRecord={onSelect}
  />
</div>

<style lang="postcss">
  @reference "../../app.css";
  .eval-timeline-markers {
    @apply absolute inset-y-0 left-0 right-24 pointer-events-none;
    --duration-gutter: 4rem;
    --outside-record-slot: 2rem;
  }
  .eval-timeline-markers :global(.timeline-marker),
  .eval-timeline-markers :global(.activity-popover) {
    pointer-events: auto;
  }
</style>
