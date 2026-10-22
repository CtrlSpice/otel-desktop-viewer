<script lang="ts">
  import type { LogData, SpanNode } from '@/types/api-types'
  import { telemetryAPI } from '@/services/telemetry-service'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
  import type { EvalTelemetrySummary } from './eval-telemetry-summaries'

  let {
    entries,
    nodes,
    loading = false,
    wholeTraces = false,
    evidenceLogs,
  }: {
    entries: EvalTelemetrySummary[]
    nodes?: Map<string, SpanNode>
    loading?: boolean
    wholeTraces?: boolean
    evidenceLogs?: LogData[]
  } = $props()
  let traceCount = $state<number>()
  let logRefs = $state<string[]>()
  const selection = $derived(
    JSON.stringify(entries.map(entry => entry.reference))
  )
  const logCount = $derived(
    logRefs && evidenceLogs
      ? new Set([...logRefs, ...evidenceLogs.map(log => log.logRef)]).size
      : undefined
  )
  const counts = $derived(
    [
      traceCount === undefined
        ? undefined
        : `${traceCount} ${traceCount === 1 ? 'trace' : 'traces'}`,
      logCount === undefined
        ? undefined
        : `${logCount} ${logCount === 1 ? 'log' : 'logs'}`,
    ]
      .filter(value => value !== undefined)
      .join(' | ')
  )
  $effect(() => {
    void nodes
    const references = JSON.parse(
      selection
    ) as EvalTelemetrySummary['reference'][]
    const full = wholeTraces
    const controller = new AbortController()
    logRefs = undefined
    void (async () => {
      const ids = [
        ...new Set(
          references.map(reference => reference.traceID.toLowerCase())
        ),
      ]
      const received = new Set<string>()
      try {
        for (let offset = 0; offset < ids.length; offset += 4) {
          const groups = await Promise.all(
            ids.slice(offset, offset + 4).map(async traceID => {
              const logs = await telemetryAPI.getTraceLogSummaries(
                traceID,
                controller.signal
              )
              const refs = references.filter(
                reference => reference.traceID.toLowerCase() === traceID
              )
              return logs.filter(
                log =>
                  full ||
                  refs.some(
                    reference =>
                      !reference.spanID ||
                      reference.spanID.toLowerCase() ===
                        log.spanID?.toLowerCase()
                  )
              )
            })
          )
          if (controller.signal.aborted) return
          for (const log of groups.flat()) received.add(log.logRef)
        }
        logRefs = [...received]
      } catch {
        // A failed lookup is unavailable, not a received count of zero.
      }
    })()
    return () => controller.abort()
  })
</script>

<DetailBubble
  label="Telemetry"
  disabled={entries.length === 0}
  initialOpen={entries.length > 0}
>
  {#snippet trailing()}
    {#if entries.length && (traceCount !== undefined || logCount !== undefined)}
      <span class="text-sm text-rp-subtle">[{counts}]</span>
    {/if}
  {/snippet}
  {#if entries.length}
    <EvalTelemetrySummaries
      {entries}
      {nodes}
      {loading}
      ontracecount={count => (traceCount = count)}
    />
  {/if}
</DetailBubble>
