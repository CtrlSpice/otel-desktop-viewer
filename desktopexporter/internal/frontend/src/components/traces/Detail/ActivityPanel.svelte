<script lang="ts">
  import { tick } from 'svelte'
  import type { EventData, TraceLogSummary } from '@/types/api-types'
  import LogCard from '@/components/logs/LogCard.svelte'
  import EventsPanel from './EventsPanel.svelte'
  import { formatSignedDuration } from '@/utils/time'
  import { navigateToItem } from '@/route'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { toolLogProvenance } from '@/utils/tool-event'

  type Props = {
    events: EventData[]
    logs: TraceLogSummary[]
    spanStartTime: bigint
    spanKey?: string
    selectedEventIndex?: number | null
    selectedLogRef?: string | null
  }

  let {
    events,
    logs,
    spanStartTime,
    spanKey,
    selectedEventIndex = null,
    selectedLogRef = null,
  }: Props = $props()

  let logProvenance = $state(
    new Map<string, NonNullable<ReturnType<typeof toolLogProvenance>>>()
  )

  $effect(() => {
    logProvenance = new Map()
    if (
      !spanKey ||
      !logs.some(log =>
        ['tool.execution', 'tool.execution.start'].includes(log.eventName)
      )
    )
      return
    const [traceID, spanID] = spanKey.split('/')
    if (!traceID || !spanID) return
    const controller = new AbortController()
    void telemetryAPI
      .getSpanLogs(traceID, spanID, controller.signal)
      .then(records => {
        if (controller.signal.aborted) return
        const entries = new Map<
          string,
          NonNullable<ReturnType<typeof toolLogProvenance>>
        >()
        for (const record of records) {
          if (record.traceID !== traceID || record.spanID !== spanID) continue
          const provenance = toolLogProvenance(record)
          if (provenance)
            entries.set(record.logRef, {
              ...provenance,
              description: `${provenance.description} · Event name: ${record.eventName}`,
            })
        }
        logProvenance = entries
      })
      .catch(error => {
        if (!controller.signal.aborted)
          console.error('Failed to load activity provenance:', error)
      })
    return () => controller.abort()
  })

  $effect(() => {
    const logRef = selectedLogRef
    if (logRef === null) return
    void tick().then(() => {
      document
        .getElementById(`span-log-${logRef}`)
        ?.scrollIntoView?.({ block: 'start' })
    })
  })
</script>

<EventsPanel {events} {spanStartTime} {spanKey} {selectedEventIndex} />

{#each logs as log (log.logRef)}
  {@const provenance = logProvenance.get(log.logRef)}
  <section
    id={`span-log-${log.logRef}`}
    class="w-full"
    aria-label="Log summary"
  >
    <LogCard
      {log}
      activity={{
        name: provenance?.name || log.eventName || log.bodyPreview,
        ...provenance,
      }}
      showLogBadge
      offset={formatSignedDuration(log.timestamp - spanStartTime)}
      selected={log.logRef === selectedLogRef}
      onclick={logRef => navigateToItem('logs', logRef, 'push')}
    />
  </section>
{/each}
