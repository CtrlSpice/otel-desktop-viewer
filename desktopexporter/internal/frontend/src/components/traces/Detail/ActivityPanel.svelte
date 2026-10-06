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

  function logOpen(logRef: string, index: number): boolean {
    if (selectedLogRef !== null) return logRef === selectedLogRef
    return selectedEventIndex === null && events.length === 0 && index === 0
  }

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

<FieldGroup label="Events" count={events.length} detail open>
  {#if events.length > 0}
    <EventsPanel {events} {spanStartTime} {selectedEventIndex} />
  {/if}
</FieldGroup>

<FieldGroup label="Logs" count={logs.length} detail open>
  {#each logs as log, index (log.logRef)}
    <div id={`span-log-${log.logRef}`}>
      <FieldGroup
        label={log.eventName || 'Log'}
        badge={formatSignedDuration(log.timestamp - spanStartTime)}
        detail
        open={logOpen(log.logRef, index)}
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
          href={itemHref('logs', log.logRef)}
          onclick={event => openLog(event, log.logRef)}>Open log</a
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
