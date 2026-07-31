<script lang="ts">
  import { tick } from 'svelte'
  import type { EventData } from '@/types/api-types'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import SpanField from './SpanField.svelte'
  import { formatDuration, formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    events: EventData[]
    spanStartTime: bigint
    selectedEventIndex?: number | null
  }

  let { events, spanStartTime, selectedEventIndex = null }: Props = $props()

  let timeContext = getTimeContext()

  function eventFieldCount(event: EventData): number {
    let n = 1
    n += event.attributes.length
    if (event.droppedAttributesCount > 0) n++
    return n
  }

  function isEventOpen(index: number): boolean {
    if (selectedEventIndex !== null) return index === selectedEventIndex
    return index === 0
  }

  $effect(() => {
    const index = selectedEventIndex
    if (index === null) return
    void tick().then(() => {
      document
        .getElementById(`span-event-${index}`)
        ?.scrollIntoView({ block: 'nearest' })
    })
  })
</script>

{#each events as event, index (index)}
  <div id={`span-event-${index}`}>
    <FieldGroup
      label={event.name}
      badge={`+${formatDuration(event.timestamp - spanStartTime)}`}
      count={eventFieldCount(event)}
      open={isEventOpen(index)}
    >
      <table class="detail-fields w-full" aria-label="Event {event.name}">
        <tbody>
          <SpanField
            fieldName="timestamp"
            fieldValue={formatTimestamp(
              event.timestamp,
              timeContext.tz,
              'nanoseconds'
            )}
            fieldType="timestamp"
          />
          {#each event.attributes as attr (attr.key)}
            <SpanField
              fieldName={attr.key}
              fieldValue={attr.value}
              fieldType={attr.type}
            />
          {/each}
          {#if event.droppedAttributesCount > 0}
            <SpanField
              fieldName="dropped attributes count"
              fieldValue={event.droppedAttributesCount.toString()}
              fieldType="uint32"
            />
          {/if}
        </tbody>
      </table>
    </FieldGroup>
  </div>
{/each}
