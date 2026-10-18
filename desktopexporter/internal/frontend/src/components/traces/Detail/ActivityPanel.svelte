<script lang="ts">
  import { tick } from 'svelte'
  import type { EventData, TraceLogSummary } from '@/types/api-types'
  import LogField from '@/components/shared/DetailField.svelte'
  import EventsPanel from './EventsPanel.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatSignedDuration, formatTimestamp } from '@/utils/time'
  import { isPlainLeftClick, itemHref, navigateToItem } from '@/route'

  type Props = {
    events: EventData[]
    logs: TraceLogSummary[]
    spanStartTime: bigint
    selectedEventIndex?: number | null
    selectedLogRef?: string | null
  }

  let {
    events,
    logs,
    spanStartTime,
    selectedEventIndex = null,
    selectedLogRef = null,
  }: Props = $props()

  const timeContext = getTimeContext()

  function openLog(event: MouseEvent, logRef: string) {
    if (!isPlainLeftClick(event)) return
    event.preventDefault()
    navigateToItem('logs', logRef, 'push')
  }

  $effect(() => {
    const logRef = selectedLogRef
    if (logRef === null) return
    void tick().then(() => {
      document
        .getElementById(`span-log-${logRef}`)
        ?.scrollIntoView?.({ block: 'nearest' })
    })
  })
</script>

<EventsPanel {events} {spanStartTime} {selectedEventIndex} />

{#each logs as log (log.logRef)}
  <table
    id={`span-log-${log.logRef}`}
    class="detail-fields w-full"
    aria-label="Log summary"
  >
    <caption class="py-2 text-left">
      <span class="badge-count mr-2">Log</span>
      {#if log.eventName}<span class="mr-2 font-medium">{log.eventName}</span
        >{/if}
      <span class="badge-count mr-2"
        >{formatSignedDuration(log.timestamp - spanStartTime)}</span
      >
      <a
        class="link link-primary text-xs"
        href={itemHref('logs', log.logRef)}
        onclick={event => openLog(event, log.logRef)}>Open log</a
      >
    </caption>
    <tbody>
      <LogField
        fieldName="timestamp"
        fieldType="timestamp"
        fieldValue={formatTimestamp(
          log.timestamp,
          timeContext.tz,
          'nanoseconds'
        )}
      />
      <LogField
        fieldName="offset"
        fieldType="duration"
        fieldValue={formatSignedDuration(log.timestamp - spanStartTime)}
      />
      {#if log.severityText}
        <LogField
          fieldName="severity text"
          fieldType="string"
          fieldValue={log.severityText}
        />
      {/if}
      <LogField
        fieldName="severity number"
        fieldType="enum"
        fieldValue={String(log.severityNumber)}
      />
      {#if log.serviceName}
        <LogField
          fieldName="service"
          fieldType="string"
          fieldValue={log.serviceName}
        />
      {/if}
      {#if log.eventName}
        <LogField
          fieldName="event name"
          fieldType="string"
          fieldValue={log.eventName}
        />
      {/if}
      {#if log.bodyPreview}
        <LogField
          fieldName="body preview"
          fieldType="string"
          fieldValue={log.bodyPreview}
          multiline
        />
      {/if}
    </tbody>
  </table>
{/each}
