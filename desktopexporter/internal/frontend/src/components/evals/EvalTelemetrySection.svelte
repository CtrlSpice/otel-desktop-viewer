<script lang="ts">
  import { untrack } from 'svelte'
  import { createResourceMap } from '@/services/resources'
  import type { LogData, SpanNode } from '@/types/api-types'
  import { telemetryAPI } from '@/services/telemetry-service'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
  import type { EvalTelemetrySummary } from './eval-telemetry-summaries'
  import type { EvalRequestStatus } from './eval-request-state'

  let {
    entries,
    nodes,
    loading = false,
    status,
    wholeTraces = false,
    evidenceLogs,
  }: {
    entries: EvalTelemetrySummary[]
    nodes?: Map<string, SpanNode>
    loading?: boolean
    status?: ReadonlyMap<string, EvalRequestStatus>
    wholeTraces?: boolean
    evidenceLogs?: LogData[]
  } = $props()
  let traceCount = $state<number>()
  const selection = $derived(
    JSON.stringify([wholeTraces, entries.map(entry => entry.reference)])
  )
  let lastLogCount = $state.raw<{ selection: string; count: number }>()
  const logCount = $derived(
    lastLogCount?.selection === selection ? lastLogCount.count : undefined
  )
  const reads = createResourceMap<string, string[]>()
  let logPartsSelection = $state('')
  let logRequestKey = $state(0)
  const logsPending = $derived(reads.pending)
  const logsFailed = $derived([...reads.values()].some(read => read.error))
  $effect(() => {
    if (!evidenceLogs || logPartsSelection !== selection) return
    const refs = new Set(evidenceLogs.map(log => log.logRef))
    for (const read of reads.values()) {
      if (read.pending || read.error || !read.data) return
      for (const ref of read.data) refs.add(ref)
    }
    lastLogCount = { selection, count: refs.size }
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
  const requests = new Map<
    string,
    {
      selection: string
      full: boolean
      nodes: SpanNode[]
    }
  >()
  $effect(() => {
    void selection
    const references = untrack(() =>
      entries.map(entry => ({ ...entry.reference }))
    )
    const full = wholeTraces
    const ids = [
      ...new Set(references.map(reference => reference.traceID.toLowerCase())),
    ]
    const selectedNodes = [...(nodes?.values() ?? [])]
    for (const [id, request] of requests) {
      if (ids.includes(id)) continue
      requests.delete(id)
      reads.delete(id)
    }
    for (const traceID of ids) {
      const refs = references.filter(
        reference => reference.traceID.toLowerCase() === traceID
      )
      const selected = JSON.stringify(refs)
      const traceNodes = selectedNodes.filter(
        node => node.spanData.traceID.toLowerCase() === traceID
      )
      const previous = requests.get(traceID)
      if (
        previous?.selection === selected &&
        previous.full === full &&
        previous.nodes.length === traceNodes.length &&
        previous.nodes.every((node, index) => node === traceNodes[index])
      )
        continue
      requests.set(traceID, {
        selection: selected,
        full,
        nodes: traceNodes,
      })
      logRequestKey = untrack(() => logRequestKey) + 1
      void reads.for(traceID).load(async signal => {
        const logs = await telemetryAPI.getTraceLogSummaries(traceID, signal)
        return logs
          .filter(
            log =>
              full ||
              refs.some(
                reference =>
                  !reference.spanID ||
                  reference.spanID.toLowerCase() === log.spanID?.toLowerCase()
              )
          )
          .map(log => log.logRef)
      })
    }
    logPartsSelection = selection
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
    <LoadingStatus
      pending={logsPending}
      requestKey={logRequestKey}
      label="Loading log count…"
      class="block px-3 py-2 text-sm text-subtle"
    />
    {#if logsFailed}<p class="px-3 py-2 text-sm text-subtle" role="status">
        Log count could not be loaded.
      </p>{/if}
    <EvalTelemetrySummaries
      {entries}
      {nodes}
      {loading}
      {status}
      ontracecount={count => {
        traceCount = count
      }}
    />
  {/if}
</DetailBubble>
