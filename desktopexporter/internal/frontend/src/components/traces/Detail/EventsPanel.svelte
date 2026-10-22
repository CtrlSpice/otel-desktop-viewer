<script lang="ts">
  import { tick } from 'svelte'
  import type { EventData } from '@/types/api-types'
  import SpanField from '@/components/shared/DetailProperty.svelte'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'
  import DisclosureSignalCard from '@/components/shared/DisclosureSignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import { formatTimestamp } from '@/utils/time'
  import {
    toolEventProvenance,
    toolEventError,
    toolEventName,
  } from '@/utils/tool-event'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    events: EventData[]
    spanStartTime: bigint
    spanKey?: string
    selectedEventIndex?: number | null
  }

  let {
    events,
    spanStartTime,
    spanKey,
    selectedEventIndex = null,
  }: Props = $props()

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
  {@const name = toolEventName(event)}
  {@const provenance = toolEventProvenance(event)}
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
      identity={JSON.stringify([
        spanKey,
        spanStartTime.toString(),
        index,
        event.name,
        event.timestamp.toString(),
      ])}
      title={name}
      description={message?.kind === 'string' ? message.value : undefined}
      reveal={selectedEventIndex === index}
    >
      {#snippet titleContent()}
        <span
          class="inline-flex max-w-full flex-wrap items-center gap-x-2 text-base-content"
        >
          {#if exception}<span
              class="size-2 shrink-0 rounded-full bg-error"
              aria-hidden="true"
            ></span>{/if}
          <span class="truncate">{name}</span>
          <span class="shrink-0 text-sm text-secondary"
            ><span class="text-rp-subtle" aria-hidden="true">{'· '}</span><span
              >Span event</span
            ></span
          >
          {#if provenance}
            <span class="shrink-0 text-sm text-rp-subtle" aria-hidden="true"
              >·</span
            >
            <span
              class="shrink-0 text-sm text-secondary"
              title={provenance.description}>{provenance.source}</span
            >
          {/if}
          {#if toolEventError(event)}<span class="shrink-0 text-sm"
              >· Tool error</span
            >{/if}
        </span>
      {/snippet}
      {#snippet date()}<SignalDate
          label="Recorded"
          timestamp={event.timestamp}
          relativeTo={spanStartTime}
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
          {#if name !== event.name}
            <SpanField
              fieldName="name"
              fieldType="string"
              fieldValue={event.name}
            />
          {/if}
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
