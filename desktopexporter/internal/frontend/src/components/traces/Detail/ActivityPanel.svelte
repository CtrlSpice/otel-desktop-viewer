<script lang="ts">
  import { tick } from 'svelte'
  import type { EventData, TraceLogSummary } from '@/types/api-types'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import LogField from '@/components/logs/LogField.svelte'
  import EventsPanel from './EventsPanel.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatSignedDuration, formatTimestamp } from '@/utils/time'
  import { isPlainLeftClick, itemHref, navigateToItem } from '@/route'

  type Props = {
    events: EventData[]
    logs: TraceLogSummary[]
    spanStartTime: bigint
    selectedEventIndex?: number | null
    selectedLogID?: string | null
  }

  let {
    events,
    logs,
    spanStartTime,
    selectedEventIndex = null,
    selectedLogID = null,
  }: Props = $props()

  const timeContext = getTimeContext()

  function logOpen(logID: string, index: number): boolean {
    if (selectedLogID !== null) return logID === selectedLogID
    return selectedEventIndex === null && events.length === 0 && index === 0
  }

  function openLog(event: MouseEvent, logID: string) {
    if (!isPlainLeftClick(event)) return
    event.preventDefault()
    navigateToItem('logs', logID, 'push')
  }

  $effect(() => {
    const logID = selectedLogID
    if (logID === null) return
    void tick().then(() => {
      document
        .getElementById(`span-log-${logID}`)
        ?.scrollIntoView?.({ block: 'nearest' })
    })
  })
</script>

<FieldGroup label="Events" count={events.length} detail open>
  {#if events.length > 0}
    <EventsPanel {events} {spanStartTime} {selectedEventIndex} />
  {/if}
</FieldGroup>

<FieldGroup label="Logs" count={logs.length} detail open>
  {#each logs as log, index (log.id)}
    <div id={`span-log-${log.id}`}>
      <FieldGroup
        label={log.eventName || 'Log'}
        badge={formatSignedDuration(log.timestamp - spanStartTime)}
        detail
        open={logOpen(log.id, index)}
      >
        <table class="detail-fields w-full" aria-label="Log summary">
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
        <a
          class="activity-panel__open-log link link-primary"
          href={itemHref('logs', log.id)}
          onclick={event => openLog(event, log.id)}>Open log</a
        >
      </FieldGroup>
    </div>
  {/each}
</FieldGroup>

<style lang="postcss">
  @reference "../../../app.css";

  .activity-panel__open-log {
    @apply inline-block py-1 text-xs;
  }
</style>
