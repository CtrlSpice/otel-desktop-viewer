<script lang="ts">
  import type { LogData, SpanData, SpanNode } from '@/types/api-types'
  import { telemetryAPI } from '@/services/telemetry-service'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import {
    evalSpanEvidence,
    toolEvidenceSpans,
    uniqueToolCallID,
    toolLogOwners,
  } from './eval-span-evidence'

  let {
    span,
    nodes,
    onlogs,
  }: {
    span: SpanData
    nodes: Map<string, SpanNode>
    onlogs?: (logs: LogData[] | undefined) => void
  } = $props()
  const spans = $derived(toolEvidenceSpans(span, nodes))
  const callID = $derived(uniqueToolCallID(span, nodes))
  const logOwners = $derived(toolLogOwners(spans, nodes, callID))
  let logs = $state<LogData[]>([])
  let loading = $state(false)
  let failed = $state(false)
  const outputField = (key: string) =>
    /^(output|stdout|stderr|tool\.(output|result|error)|gen_ai\.tool\.call\.result)$/.test(
      key
    )
  const records = $derived(
    evalSpanEvidence(spans, logs, callID)
      .map(record => ({
        ...record,
        fields: [...record.fields].sort(
          (a, b) => Number(outputField(b.key)) - Number(outputField(a.key))
        ),
      }))
      .sort(
        (a, b) =>
          Number(b.fields.some(field => outputField(field.key))) -
          Number(a.fields.some(field => outputField(field.key)))
      )
  )

  $effect(() => {
    const selected = logOwners
    const controller = new AbortController()
    logs = []
    loading = true
    failed = false
    void (async () => {
      const received: LogData[] = []
      let unavailable = false
      for (let offset = 0; offset < selected.length; offset += 4) {
        const results = await Promise.allSettled(
          selected
            .slice(offset, offset + 4)
            .map(item =>
              telemetryAPI.getSpanLogs(
                item.traceID,
                item.spanID,
                controller.signal
              )
            )
        )
        if (controller.signal.aborted) return
        for (const result of results) {
          if (result.status === 'fulfilled') received.push(...result.value)
          else unavailable = true
        }
      }
      logs = received
      failed = unavailable
      loading = false
    })()
    return () => controller.abort()
  })
  $effect(() => {
    const ids = new Set(
      records
        .filter(record => record.id.startsWith('log/'))
        .map(record => record.id.slice(4))
    )
    onlogs?.(
      loading || failed ? undefined : logs.filter(log => ids.has(log.logRef))
    )
  })
</script>

{#each records as record (record.id)}
  <table class="detail-fields w-full" aria-label={record.source}>
    <caption
      title={record.sourceDescription}
      class="px-[var(--pane-inline-inset)] py-2 text-left text-sm font-normal text-secondary"
      >{record.source}{#if record.toolError}<span class="text-subtle">
          ·
        </span><span class="text-base-content">Tool error</span>{/if}</caption
    >
    <tbody>
      {#each record.fields as attribute, index (`${attribute.key}/${index}`)}
        <DetailField
          fieldName={attribute.key}
          fieldType={attribute.value.kind}
          typedValue={attribute.value}
          fieldKind="attribute"
          defectLabel={attribute.hasConflict
            ? `Conflicting typed values retained for ${attribute.key}.`
            : undefined}
        />
      {/each}
    </tbody>
  </table>
{/each}
{#if loading}<p class="px-3 py-2 text-sm text-subtle" role="status">
    Loading linked log evidence…
  </p>
{:else if failed}<p class="px-3 py-2 text-sm text-subtle" role="status">
    Linked log evidence could not be loaded.
  </p>{/if}
