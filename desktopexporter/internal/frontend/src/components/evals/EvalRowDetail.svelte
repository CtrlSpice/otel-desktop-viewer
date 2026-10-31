<script lang="ts">
  import { onDestroy, untrack } from 'svelte'
  import type { EvalRequestPart, EvalRequestStatus } from './eval-request-state'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import EvalAssessmentDetail, {
    type EvalGraderInput,
  } from './EvalAssessmentDetail.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import EvalSpanEvidence from './EvalSpanEvidence.svelte'
  import MarkdownText from '@/components/shared/MarkdownText.svelte'
  import {
    cost,
    localTimestamp,
    type EvalExperiment,
    type EvalView,
  } from './model'
  import { caseSummary } from './case-summary'
  import { experimentSummary } from './experiment-summary'
  import EvalTelemetrySection from './EvalTelemetrySection.svelte'
  import { evalTelemetrySummaries } from './eval-telemetry-summaries'
  import { referenceKey } from './eval-traces'
  import type { LogData, SpanData, SpanNode } from '@/types/api-types'
  import {
    evalSpanEvidence,
    toolEvidenceSpans,
    toolLogOwners,
    uniqueToolCallID,
  } from './eval-span-evidence'
  import { completionLabel, completionTone } from './group-results'
  import type { EvalOutlineRow } from './eval-outline'
  import { formatDuration } from '@/utils/time'
  import type { EvalSourceFieldCopy } from './eval-source-copy'

  let {
    experiment,
    row,
    mode = 'results',
    telemetryNodes,
    telemetryLoading = false,
    telemetryStatus,
    sourceCopy,
  }: {
    experiment: EvalExperiment
    row: EvalOutlineRow
    mode?: EvalView
    telemetryNodes?: Map<string, SpanNode>
    telemetryLoading?: boolean
    telemetryStatus?: ReadonlyMap<string, EvalRequestStatus>
    sourceCopy?: EvalSourceFieldCopy
  } = $props()
  const requested = $derived(
    JSON.stringify([
      experiment.id,
      row.id,
      row.kind,
      row.reference && referenceKey(row.reference),
    ])
  )
  const copySource = $derived({ source: sourceCopy, selection: requested })
  const spanRequest = $derived.by(() => {
    const nodes = telemetryNodes
    const reference = row.reference
    const selectedSpan =
      reference &&
      nodes &&
      ['tool', 'span', 'grader', 'execution'].includes(row.kind)
        ? nodes.get(referenceKey(reference))?.spanData
        : undefined
    const spans =
      selectedSpan && nodes ? toolEvidenceSpans(selectedSpan, nodes) : []
    const callID =
      selectedSpan && nodes ? uniqueToolCallID(selectedSpan, nodes) : undefined
    return {
      context: requested,
      row,
      spans,
      callID,
      selectedSpan,
      applicable:
        !!reference?.spanID &&
        ['tool', 'span', 'grader', 'execution'].includes(row.kind),
      owners: nodes ? toolLogOwners(spans, nodes, callID) : [],
    }
  })
  const spanStatus = $derived(
    row.reference && telemetryStatus
      ? (telemetryStatus.get(row.reference.traceID.toLowerCase()) ?? 'pending')
      : telemetryLoading
        ? 'pending'
        : undefined
  )
  let displayedSpans = $state.raw<typeof spanRequest>()
  $effect(() => {
    const next = spanRequest
    if (!next.applicable || next.selectedSpan || spanStatus === 'ready')
      displayedSpans = next
  })
  const spanEvidence = $derived(evalSpanEvidence(displayedSpans?.spans ?? []))
  type LogPacket = {
    request: typeof spanRequest
    owner: SpanData
    logs: LogData[]
  }
  let retainedLogs = $state.raw<LogPacket[]>([])
  let logRequest = $state.raw<{
    request: typeof spanRequest
    parts: Map<string, EvalRequestPart<LogData[]>>
  }>()
  const evidenceLoading = $derived(
    logRequest?.request.context === requested &&
      [...logRequest.parts.values()].some(part => part.status === 'pending')
  )
  const evidenceFailed = $derived(
    logRequest?.request.context === requested &&
      [...logRequest.parts.values()].some(part => part.status === 'failed')
  )
  let requestKey = $state.raw<AbortSignal>()
  const visibleLogPackets = $derived.by(() => {
    const current = logRequest
    if (!current) return retainedLogs
    const ready = current.request.owners.flatMap(owner => {
      const part = current.parts.get(referenceKey(owner))
      return part?.status === 'ready'
        ? [{ request: current.request, owner, logs: part.value }]
        : []
    })
    const unresolved = [...current.parts.values()].some(
      part => part.status !== 'ready'
    )
    return [
      ...ready,
      ...retainedLogs.filter(packet => {
        const replacement = current.parts.get(referenceKey(packet.owner))
        return replacement ? replacement.status !== 'ready' : unresolved
      }),
    ]
  })
  function packetEvidence(packet: LogPacket) {
    return evalSpanEvidence(
      packet.request.spans,
      packet.logs,
      packet.request.callID
    ).filter(record => record.id.startsWith('log/'))
  }
  const evidenceLogs = $derived.by(() => {
    if (
      !logRequest ||
      logRequest.request.context !== requested ||
      evidenceLoading ||
      evidenceFailed
    )
      return undefined
    const received = new Map<string, LogData>()
    for (const packet of visibleLogPackets) {
      if (packet.request !== logRequest.request) continue
      const ids = new Set(
        packetEvidence(packet).map(record => record.id.slice(4))
      )
      for (const log of packet.logs)
        if (ids.has(log.logRef)) received.set(log.logRef, log)
    }
    return [...received.values()]
  })

  // Logs retain only their own packet. Span attributes/events publish separately.
  let logController: AbortController | undefined
  onDestroy(() => logController?.abort())
  $effect(() => {
    const selected = spanRequest
    const waiting =
      selected.applicable && !selected.selectedSpan && spanStatus !== 'ready'
    const previous = untrack(() => logRequest?.request)
    if (
      logController &&
      !logController.signal.aborted &&
      previous &&
      previous.context === selected.context &&
      previous.callID === selected.callID &&
      previous.spans.length === selected.spans.length &&
      previous.spans.every((span, index) => span === selected.spans[index]) &&
      previous.owners.length === selected.owners.length &&
      previous.owners.every((span, index) => span === selected.owners[index])
    )
      return
    logController?.abort()
    if (waiting) return
    const { owners } = selected
    retainedLogs = owners.length ? untrack(() => visibleLogPackets) : []
    const parts = new Map<string, EvalRequestPart<LogData[]>>(
      owners.map(owner => [referenceKey(owner), { status: 'pending' }])
    )
    logRequest = { request: selected, parts: new Map(parts) }
    const controller = new AbortController()
    logController = controller
    requestKey = controller.signal
    for (const owner of owners) {
      void telemetryAPI
        .getSpanLogs(owner.traceID, owner.spanID, controller.signal)
        .then(
          logs => publish(owner, { status: 'ready', value: logs }),
          () => publish(owner, { status: 'failed' })
        )
    }
    function publish(owner: SpanData, part: EvalRequestPart<LogData[]>) {
      if (controller.signal.aborted) return
      parts.set(referenceKey(owner), part)
      logRequest = { request: selected, parts: new Map(parts) }
    }
  })
  const result = $derived(row.result)
  const references = $derived(evalTelemetrySummaries(experiment, row))
  const members = $derived(
    experiment.results.filter(item => item.caseID === row.caseID)
  )
  const summary = $derived(caseSummary(members, experiment.assessments))
  const outcomes = $derived(
    experimentSummary({ ...experiment, results: members }).parts
  )
  const tool = $derived(
    result?.tools.find(
      (tool, index) =>
        row.id === `${result.id}/tool/${index}` ||
        (row.kind === 'tool' &&
          row.reference &&
          tool.telemetry &&
          referenceKey(row.reference) === referenceKey(tool.telemetry))
    )
  )
  const assessments = $derived(
    result?.assessments.filter(item =>
      row.kind === 'result' || row.kind === 'case'
        ? mode === 'results'
        : row.kind === 'grading' || item.id === row.assessmentID
    ) ?? []
  )
  const prose = $derived(
    mode === 'cost'
      ? undefined
      : row.kind === 'case' && !result
        ? mode === 'results'
          ? members[0]?.question
          : undefined
        : row.kind === 'result' || row.kind === 'case'
          ? mode === 'results'
            ? result?.answer
            : undefined
          : row.kind === 'execution'
            ? result?.answer
            : tool?.message
  )
  const overallDefinition = $derived(
    experiment.assessments.find(
      item =>
        item.id === experiment.headlineAssessment && item.name === 'Overall'
    )
  )
  const overall = $derived(
    mode === 'results'
      ? assessments.find(item => item.id === overallDefinition?.id)
      : undefined
  )
  let overallInput = $state.raw<EvalGraderInput>()
  const proseCopy = $derived(
    row.kind === 'case' && !result && members[0]
      ? sourceCopy?.(members[0].id, 'question')
      : (row.kind === 'case' ||
            row.kind === 'result' ||
            row.kind === 'execution') &&
          result
        ? sourceCopy?.(result.id, 'answer')
        : undefined
  )
  const span = $derived(
    row.reference
      ? telemetryNodes?.get(referenceKey(row.reference))?.spanData
      : undefined
  )
  const duration = $derived(
    row.window
      ? formatDuration(row.window.endTime - row.window.startTime)
      : row.seconds !== undefined
        ? `${row.seconds} s`
        : undefined
  )
  const durationSource = $derived(
    row.window
      ? span && span.endTime >= span.startTime
        ? 'Recorded span'
        : 'Derived from spans'
      : undefined
  )
  const showDuration = $derived(
    row.window
      ? row.window.endTime - row.window.startTime > 1n
      : row.seconds !== undefined && row.seconds > 1e-9
  )
  const costFields = $derived(
    row.kind === 'case' || row.kind === 'result'
      ? [
          {
            name: 'investigator API estimate (CAD)',
            estimate:
              row.kind === 'case'
                ? summary.investigatorCost
                : {
                    total: result?.investigatorCost ?? null,
                    incomplete: false,
                  },
          },
          {
            name: 'grader API estimate (CAD)',
            estimate:
              row.kind === 'case'
                ? summary.graderCost
                : { total: result?.graderCost ?? null, incomplete: false },
          },
          { name: 'total API estimate (CAD)', estimate: row.estimate },
        ]
      : [{ name: 'API estimate (CAD)', estimate: row.estimate }]
  )
  const calls = $derived(row.toolCounts?.total ?? 0)
  const failures = $derived(row.toolCounts?.failed ?? 0)
</script>

{#snippet detailsSection()}
  <DetailBubble
    label={row.name}
    layoutKey={row.kind === 'case' ? 'Question' : 'Result'}
  >
    {#snippet trailing()}
      {#if row.kind === 'case'}
        <span
          class="inline-flex flex-wrap items-center justify-end gap-4"
          role="list"
          aria-label="Test case outcomes"
        >
          {#each outcomes as outcome}
            <span
              class="inline-flex items-center gap-2 whitespace-nowrap"
              role="listitem"
            >
              <EvalStatus label={outcome.label} tone={outcome.tone} />
              <span class="text-subtle tabular-nums">[{outcome.count}]</span>
            </span>
          {/each}
        </span>
      {:else}
        <EvalStatus
          label={completionLabel(row.state, row.internalFault)}
          tone={completionTone(row.state, row.internalFault)}
        />
      {/if}
    {/snippet}
    <div hidden={mode === 'cost'}>
      <EvalSpanEvidence
        evidence={spanEvidence}
        owner={displayedSpans?.context !== requested
          ? displayedSpans?.row.name
          : undefined}
        reference={displayedSpans?.row.reference}
      />
      {#each visibleLogPackets as packet (`${referenceKey(packet.owner)}/${packet.request.row.id}`)}
        <EvalSpanEvidence
          evidence={packetEvidence(packet)}
          owner={packet.request.context !== requested
            ? packet.request.row.name
            : undefined}
          reference={packet.request.row.reference}
        />
      {/each}
      {#if spanRequest.applicable && spanStatus === 'failed'}
        <p class="px-3 py-2 text-sm text-subtle" role="status">
          Referenced span could not be loaded.
        </p>
      {/if}
      {#if evidenceLoading}
        <LoadingStatus
          pending
          label="Loading linked log evidence…"
          {requestKey}
          class="block px-3 py-2 text-sm text-subtle"
        />
      {:else if evidenceFailed}
        <p class="px-3 py-2 text-sm text-subtle" role="status">
          Linked log evidence could not be loaded.
        </p>
      {/if}
    </div>
    {#if mode === 'cost'}
      <table class="detail-fields w-full" aria-label={`${row.name} accounting`}>
        <tbody>
          {#each costFields as field}
            <DetailField
              fieldName={field.name}
              fieldType="number"
              fieldValue={`${cost(field.estimate.total)}${field.estimate.incomplete && field.estimate.total !== null ? ' · partial' : ''}`}
            />
          {/each}
        </tbody>
      </table>
    {:else}
      {#if row.kind === 'case' && result && mode === 'results'}
        <table class="detail-fields w-full" aria-label="Test case input">
          <tbody>
            <DetailField
              fieldName="input"
              fieldType="string"
              fieldValue={result.question}
              loadText={sourceCopy?.(result.id, 'question')}
              sourceKey={copySource}
              multiline
            />
          </tbody>
        </table>
      {/if}
      {#if prose !== undefined}
        {#if tool}
          <table class="detail-fields w-full" aria-label={`${row.name} output`}>
            <tbody
              ><DetailField
                fieldName={tool.state === 'Error' ? 'error' : 'output'}
                fieldType="string"
                fieldValue={prose}
                multiline
              /></tbody
            >
          </table>
        {:else}
          {@const fieldName =
            row.kind === 'case' && !result ? 'input' : 'output'}
          <table
            class="detail-fields w-full"
            aria-label={`${row.name} ${fieldName}`}
          >
            <tbody>
              <DetailField
                {fieldName}
                fieldType={fieldName === 'output'
                  ? (result?.answerType ?? 'string')
                  : 'string'}
                fieldValue={prose}
                loadText={proseCopy}
                sourceKey={copySource}
                multiline
              >
                {#snippet value()}
                  {#if fieldName === 'output' && result?.answerType && result.answerType !== 'string'}
                    {prose}
                  {:else}
                    <MarkdownText text={prose === '' ? '""' : prose} />
                  {/if}
                {/snippet}
              </DetailField>
            </tbody>
          </table>
        {/if}
      {:else if row.kind === 'result' && mode === 'results'}
        <p class="eval-row-detail__prose">No answer recorded</p>
      {/if}

      {#if (row.kind === 'case' || row.kind === 'result' || row.kind === 'execution') && result?.error !== undefined}
        <table class="detail-fields w-full" aria-label="Result fault">
          <tbody
            ><DetailField
              fieldName="internal fault"
              fieldType="string"
              fieldValue={result.error}
              loadText={sourceCopy?.(result.id, 'error')}
              sourceKey={copySource}
              multiline
            /></tbody
          >
        </table>
      {/if}
    {/if}

    <table class="detail-fields w-full" aria-label={`${row.name} fields`}>
      <tbody>
        {#if (row.kind === 'result' || row.kind === 'case') && result?.sourceID}
          <DetailField
            fieldName="result ID"
            fieldType="string"
            fieldValue={result.sourceID}
            identifier
          />
        {/if}
        {#if row.kind === 'case' && !result}
          <DetailField
            fieldName="results"
            fieldType="number"
            fieldValue={String(members.length)}
          />
        {/if}
        {#if mode === 'time' && row.toolCounts && ['case', 'result', 'execution'].includes(row.kind)}
          <DetailField
            fieldName="tool calls"
            fieldType="number"
            fieldValue={result?.toolsKnown === false
              ? 'Unknown'
              : `${calls} total · ${failures} failed`}
          />
        {/if}
        {#if mode !== 'cost'}
          {#if (row.kind === 'case' || row.kind === 'result') && result?.startedAt !== undefined}
            <DetailField
              fieldName="started"
              fieldType="timestamp"
              fieldValue={result.startedAt}
              timestampLabel={localTimestamp(result.startedAt)}
            />
          {/if}
          {#if showDuration}
            <DetailField
              fieldName="duration"
              fieldType={row.window ? 'int64' : 'number'}
              fieldValue={duration}
              qualifier={durationSource}
            />
          {/if}
        {/if}
        {#if row.kind !== 'case' && mode !== 'time'}
          <DetailField
            fieldName="test case"
            fieldType="string"
            fieldValue={members[0]?.name ?? row.caseID}
            loadText={members[0]
              ? sourceCopy?.(members[0].id, 'name')
              : undefined}
            sourceKey={copySource}
          />
        {/if}
      </tbody>
    </table>
  </DetailBubble>
{/snippet}
{#snippet assessmentsSection()}
  <DetailBubble
    label="Assessments"
    action={overall && overallInput?.available ? overallInputCopy : undefined}
  >
    {#snippet trailing()}
      {#if overall}
        <span class="inline-flex items-center gap-2">
          <AssessmentBadge
            assessment={overall}
            maximum={overallDefinition?.maximum}
          />
        </span>
      {/if}
    {/snippet}
    {#each assessments as assessment (assessment.id)}
      <EvalAssessmentDetail
        {assessment}
        contextLabel={`${row.name} · ${result?.id ?? row.id}`}
        definition={experiment.assessments.find(
          item => item.id === assessment.id
        )}
        showScore={mode === 'results'}
        showHeading={assessment.id !== overall?.id}
        externalCopy={assessment.id === overall?.id}
        onGraderInput={assessment.id === overall?.id
          ? input => (overallInput = input)
          : undefined}
        loadRubric={result
          ? sourceCopy?.(result.id, `assessment:${assessment.id}:rubric`)
          : undefined}
        loadExplanation={result
          ? sourceCopy?.(
              result.id,
              assessment.id === experiment.headlineAssessment
                ? 'explanation'
                : `assessment:${assessment.id}:explanation`
            )
          : undefined}
      />
    {/each}
  </DetailBubble>
{/snippet}
{#snippet overallInputCopy()}
  {#if overallInput}
    <span
      role="group"
      aria-label={`Grader input · ${overallInput.contextLabel}`}
      title={`Grader input · ${overallInput.contextLabel}`}
    >
      <ValueCopyButton
        text={overallInput.text}
        loadText={overallInput.loadText}
        sourceKey={overallInput}
        label="grader input"
      />
    </span>
  {/if}
{/snippet}
{#snippet telemetrySection()}
  <EvalTelemetrySection
    entries={references}
    nodes={telemetryNodes}
    loading={telemetryLoading}
    status={telemetryStatus}
    wholeTraces={row.kind === 'result' || row.kind === 'case'}
    evidenceLogs={spanRequest.applicable ? evidenceLogs : []}
  />
{/snippet}
<DetailStack
  label={row.kind === 'result' ? 'Result details' : `${row.name} details`}
  storageKey={`eval-${row.kind === 'result' ? 'rep' : row.kind}-detail-layout`}
  sections={[
    { id: 'details', content: detailsSection },
    ...(mode !== 'cost' && assessments.length
      ? [{ id: 'assessments', content: assessmentsSection }]
      : []),
    { id: 'telemetry', content: telemetrySection },
  ]}
/>

<style lang="postcss">
  @reference "../../app.css";
  .eval-row-detail__prose {
    @apply py-2 text-sm text-base-content;
    padding-inline: var(--pane-inline-inset);
    overflow-wrap: anywhere;
  }
</style>
