<script lang="ts">
  import type { EventData } from '@/types/api-types'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import SpanField from './SpanField.svelte'
  import { formatDuration, formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    events: EventData[]
    spanStartTime: bigint
  }

  let { events, spanStartTime }: Props = $props()

  let timeContext = getTimeContext()

  function eventFieldCount(event: EventData): number {
    let n = 1
    n += event.attributes.length
    if (event.droppedAttributesCount > 0) n++
    return n
  }
</script>

{#each events as event, index (index)}
  <FieldGroup
    label={event.name}
    badge={`+${formatDuration(event.timestamp - spanStartTime)}`}
    count={eventFieldCount(event)}
    open={index === 0}
  >
    <table class="detail-fields w-full" aria-label="Event {event.name}">
      <tbody>
        <SpanField
          fieldName="timestamp"
          fieldValue={formatTimestamp(
            event.timestamp,
            timeContext.timezone,
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
{/each}
