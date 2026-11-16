<script lang="ts">
  import TimelineMarkerPopover from './TimelineMarkerPopover.svelte'
  import type { TimelineMarker, TimelineRecord } from './timeline-markers'

  type Props = {
    markers: TimelineMarker[]
    spanColor: string
    spanStartTime: bigint
    onSelectRecord: (record: TimelineRecord) => void
  }

  let { markers, spanColor, spanStartTime, onSelectRecord }: Props = $props()
</script>

{#each markers as marker (marker.id)}
  <TimelineMarkerPopover
    {marker}
    {spanColor}
    {spanStartTime}
    {onSelectRecord}
  />
{/each}
