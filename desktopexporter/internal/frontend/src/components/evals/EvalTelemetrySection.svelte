<script lang="ts">
  import { onDestroy, untrack } from 'svelte'
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
  let logRefs = $state<{ request: AbortSignal; refs: string[] }>()
  let logRequest = $state.raw<AbortSignal>()
  const selection = $derived(
    JSON.stringify(entries.map(entry => entry.reference))
  )
  let logCount = $state<number>()
  $effect(() => {
    if (logRefs && logRefs.request === logRequest && evidenceLogs)
      logCount = new Set([
        ...logRefs.refs,
        ...evidenceLogs.map(log => log.logRef),
      ]).size
  })
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
  let logController: AbortController | undefined
  let logScope:
    { selection: string; full: boolean; nodes: SpanNode[] } | undefined
  onDestroy(() => logController?.abort())
  $effect(() => {
    const selected = selection
    const references = untrack(() =>
      entries.map(entry => ({ ...entry.reference }))
    )
    const full = wholeTraces
    const ids = [
      ...new Set(references.map(reference => reference.traceID.toLowerCase())),
    ]
    const selectedNodes = [...(nodes?.values() ?? [])].filter(node =>
      ids.includes(node.spanData.traceID.toLowerCase())
    )
    if (
      logScope?.selection === selected &&
      logScope.full === full &&
      logScope.nodes.length === selectedNodes.length &&
      logScope.nodes.every((node, index) => node === selectedNodes[index])
    )
      return
    logScope = { selection: selected, full, nodes: selectedNodes }
    logController?.abort()
    const controller = new AbortController()
    logController = controller
    logRequest = controller.signal
    void (async () => {
      const received = new Set<string>()
      try {
        const groups = await Promise.all(
          ids.map(async traceID => {
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
                    reference.spanID.toLowerCase() === log.spanID?.toLowerCase()
                )
            )
          })
        )
        if (controller.signal.aborted) return
        for (const log of groups.flat()) received.add(log.logRef)
        logRefs = { request: controller.signal, refs: [...received] }
      } catch {
        // A failed lookup is unavailable, not a received count of zero.
      }
    })()
  })
</script>

<DetailBubble
  label="Telemetry"
  disabled={entries.length === 0}
  initialOpen={entries.length > 0}
>
  {#snippet trailing()}
    {#if entries.length && (traceCount !== undefined || logCount !== undefined)}
      <span class="text-sm text-subtle">[{counts}]</span>
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
