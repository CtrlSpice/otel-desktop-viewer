<script lang="ts">
  import { tick } from 'svelte'
  import type { EventData } from '@/types/api-types'
  import SpanField from '@/components/shared/DetailProperty.svelte'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'
  import DisclosureSignalCard from '@/components/shared/DisclosureSignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
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
        ?.scrollIntoView?.({ block: 'nearest' })
    })
  })
</script>

{#each events as event, index (index)}
  {@const exception = event.name === 'exception'}
  {@const message = event.attributes.find(
    attribute => attribute.key === 'exception.message'
  )?.value}
  <section
    id={`span-event-${index}`}
    class="w-full"
    aria-label="Event {event.name}"
  >
    <DisclosureSignalCard
      title={event.name}
      description={message?.kind === 'string' ? message.value : undefined}
      reveal={selectedEventIndex === index}
    >
      {#snippet titleContent()}
        <span
          class="inline-flex max-w-full items-center gap-2 text-base-content"
        >
          {#if exception}<span
              class="size-2 shrink-0 rounded-full bg-error"
              aria-hidden="true"
            ></span>{/if}
          <span class="truncate">{event.name}</span>
          <span class="shrink-0 text-sm text-secondary"
            ><span aria-hidden="true">{'· '}</span><span>Span event</span></span
          >
        </span>
      {/snippet}
      {#snippet date()}<SignalDate
          label="Recorded"
          timestamp={event.timestamp}
          suffix={`(${formatSignedDuration(event.timestamp - spanStartTime)})`}
        />{/snippet}
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
