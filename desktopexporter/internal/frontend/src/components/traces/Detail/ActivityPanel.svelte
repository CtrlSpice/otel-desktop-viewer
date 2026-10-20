<script lang="ts">
  import { tick } from 'svelte'
  import type { EventData, TraceLogSummary } from '@/types/api-types'
  import LogCard from '@/components/logs/LogCard.svelte'
  import EventsPanel from './EventsPanel.svelte'
  import { formatSignedDuration } from '@/utils/time'
  import { navigateToItem } from '@/route'

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
  <section
    id={`span-log-${log.logRef}`}
    class="w-full"
    aria-label="Log summary"
  >
    {#if log.eventName}
      <div class="px-3 py-2 text-left text-sm font-normal">{log.eventName}</div>
    {/if}
    <LogCard
      {log}
      showLogBadge
      offset={formatSignedDuration(log.timestamp - spanStartTime)}
      selected={log.logRef === selectedLogRef}
      onclick={logRef => navigateToItem('logs', logRef, 'push')}
    />
  </section>
{/each}
