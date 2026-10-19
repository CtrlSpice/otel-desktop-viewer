<script lang="ts">
  import { tick } from 'svelte'
  import type { EventData } from '@/types/api-types'
  import SpanField from '@/components/shared/DetailProperty.svelte'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'
  import DisclosureSignalCard from '@/components/shared/DisclosureSignalCard.svelte'
  import {
    formatSignedDuration,
    formatTimestamp,
    formatTimestampParts,
  } from '@/utils/time'
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
        ?.scrollIntoView?.({ block: 'nearest' })
    })
  })
</script>

{#each events as event, index (index)}
  {@const timestamp = formatTimestampParts(
    event.timestamp,
    timeContext.tz,
    'milliseconds'
  )}
  <section
    id={`span-event-${index}`}
    class="w-full"
    aria-label="Event {event.name}"
  >
    <DisclosureSignalCard
      title={event.name}
      timestamp={timestamp.value}
      timestampUnit={timestamp.unit || undefined}
      timestampSuffix={`(${formatSignedDuration(event.timestamp - spanStartTime)})`}
      reveal={selectedEventIndex === index}
    >
      {#snippet badge()}<span class="badge-count">Span event</span>{/snippet}
      <table class="detail-fields w-full" aria-label="Event {event.name}">
        <tbody>
          <SpanField
            fieldName="timeUnixNano"
            fieldValue={event.timestamp.toString()}
            timestampLabel={formatTimestamp(
              event.timestamp,
              timeContext.tz,
              'milliseconds'
            )}
            fieldType="fixed64"
          />
          <AttributeRows
            attributes={event.attributes}
            owner={`event ${index + 1}`}
          />
          {#if event.droppedAttributesCount > 0}
            <SpanField
              fieldName="droppedAttributesCount"
              fieldValue={event.droppedAttributesCount.toString()}
              fieldType="uint32"
            />
          {/if}
        </tbody>
      </table>
    </DisclosureSignalCard>
  </section>
{/each}
