<script lang="ts">
  import { onDestroy, untrack } from 'svelte'
  import type { LogData, SpanNode } from '@/types/api-types'
  import { telemetryAPI } from '@/services/telemetry-service'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
  import type { EvalTelemetrySummary } from './eval-telemetry-summaries'
  import type { EvalRequestPart, EvalRequestStatus } from './eval-request-state'

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
    JSON.stringify(entries.map(entry => entry.reference))
  )
  let logCount = $state<number>()
  let logParts = $state.raw<Map<string, EvalRequestPart<string[]>>>()
  let logRequestKey = $state(0)
  const logsPending = $derived(
    [...(logParts?.values() ?? [])].some(part => part.status === 'pending')
  )
  const logsFailed = $derived(
    [...(logParts?.values() ?? [])].some(part => part.status === 'failed')
  )
  $effect(() => {
    if (!evidenceLogs || !logParts) return
    const refs = new Set(evidenceLogs.map(log => log.logRef))
    for (const part of logParts.values()) {
      if (part.status !== 'ready') return
      for (const ref of part.value) refs.add(ref)
    }
    logCount = refs.size
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
      controller: AbortController
      selection: string
      full: boolean
      nodes: SpanNode[]
    }
  >()
  onDestroy(() => {
    for (const { controller } of requests.values()) controller.abort()
  })
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
    const parts = new Map(untrack(() => logParts))
    for (const [id, request] of requests) {
      if (ids.includes(id)) continue
      request.controller.abort()
      requests.delete(id)
      parts.delete(id)
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
      previous?.controller.abort()
      const controller = new AbortController()
      requests.set(traceID, {
        controller,
        selection: selected,
        full,
        nodes: traceNodes,
      })
      parts.set(traceID, { status: 'pending' })
      logRequestKey = untrack(() => logRequestKey) + 1
      void telemetryAPI.getTraceLogSummaries(traceID, controller.signal).then(
        logs =>
          publish({
            status: 'ready',
            value: logs
              .filter(
                log =>
                  full ||
                  refs.some(
                    reference =>
                      !reference.spanID ||
                      reference.spanID.toLowerCase() ===
                        log.spanID?.toLowerCase()
                  )
              )
              .map(log => log.logRef),
          }),
        () => publish({ status: 'failed' })
      )
      function publish(part: EvalRequestPart<string[]>) {
        if (controller.signal.aborted) return
        logParts = new Map(untrack(() => logParts)).set(traceID, part)
      }
    }
    logParts = parts
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
        if (count !== undefined) traceCount = count
      }}
    />
  {/if}
</DetailBubble>
