<script lang="ts">
  import { tick } from 'svelte'
  import type { EventData } from '@/types/api-types'
  import SpanField from './SpanField.svelte'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'
  import { formatSignedDuration, formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    events: EventData[]
    spanStartTime: bigint
    selectedEventIndex?: number | null
  }

  let { events, spanStartTime, selectedEventIndex = null }: Props = $props()

  let timeContext = getTimeContext()

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
  <table
    id={`span-event-${index}`}
    class="detail-fields w-full"
    aria-label="Event {event.name}"
  >
    <caption class="py-2 text-left">
      <span class="badge-count mr-2">Event</span>
      <span class="mr-2 font-medium">{event.name}</span>
      <span class="badge-count mr-2"
        >{formatSignedDuration(event.timestamp - spanStartTime)}</span
      >
      <a
        class="badge badge-xs badge-soft badge-warning"
        href="https://opentelemetry.io/blog/2026/deprecating-span-events/"
        target="_blank"
        rel="noopener noreferrer"
        title="OpenTelemetry is deprecating the Span Event API in favour of the Logs API. Existing span-event data remains supported."
        >API deprecation</a
      >
    </caption>
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
      <AttributeRows
        attributes={event.attributes}
        owner={`event ${index + 1}`}
      />
      {#if event.droppedAttributesCount > 0}
        <SpanField
          fieldName="dropped attributes count"
          fieldValue={event.droppedAttributesCount.toString()}
          fieldType="uint32"
        />
      {/if}
    </tbody>
  </table>
{/each}
