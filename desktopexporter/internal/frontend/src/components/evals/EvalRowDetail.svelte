<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import EvalAssessmentDetail from './EvalAssessmentDetail.svelte'
  import EvalSpanEvidence from './EvalSpanEvidence.svelte'
  import MarkdownText from '@/components/shared/MarkdownText.svelte'
  import {
    cost,
    evalToolCounts,
    localTimestamp,
    type EvalExperiment,
    type EvalView,
  } from './model'
  import { caseSummary } from './case-summary'
  import { experimentSummary } from './experiment-summary'
  import EvalTelemetrySection from './EvalTelemetrySection.svelte'
  import { evalTelemetrySummaries } from './eval-telemetry-summaries'
  import { referenceKey } from './eval-traces'
  import type { LogData, SpanNode } from '@/types/api-types'
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
    sourceCopy,
  }: {
    experiment: EvalExperiment
    row: EvalOutlineRow
    mode?: EvalView
    telemetryNodes?: Map<string, SpanNode>
    telemetryLoading?: boolean
    sourceCopy?: EvalSourceFieldCopy
  } = $props()
  const result = $derived(row.result)
  let evidenceLogs = $state<LogData[]>()
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
  const calls = $derived(
    row.kind === 'case'
      ? summary.calls
      : (row.toolCounts?.total ?? (result ? evalToolCounts(result).total : 0))
  )
  const failures = $derived(
    row.kind === 'case'
      ? summary.failures
      : (row.toolCounts?.failed ?? (result ? evalToolCounts(result).failed : 0))
  )
</script>

{#snippet overallInputCopy()}
  {#if overall}
    <ValueCopyButton
      text={overall.rubric}
      loadText={result
        ? sourceCopy?.(result.id, `assessment:${overall.id}:rubric`)
        : undefined}
      label="grader input"
    />
  {/if}
{/snippet}

{#snippet detailsSection()}
  <DetailBubble
    label={row.name}
    layoutKey={row.kind === 'case' ? 'Question' : row.name}
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
    {#if span && telemetryNodes && (row.kind === 'tool' || row.kind === 'span' || row.kind === 'grader' || row.kind === 'execution')}
      <div hidden={mode === 'cost'}>
        <EvalSpanEvidence
          {span}
          nodes={telemetryNodes}
          onlogs={logs => (evidenceLogs = logs)}
        />
      </div>
    {/if}
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
        {#if mode === 'time' && ['case', 'result', 'execution'].includes(row.kind)}
          <DetailField
            fieldName="tool calls"
            fieldType="number"
            fieldValue={members.some(item => item.toolsKnown === false)
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
          />
        {/if}
      </tbody>
    </table>
  </DetailBubble>
{/snippet}
{#snippet assessmentsSection()}
  <DetailBubble
    label="Assessments"
    action={overall && (overall.rubric || sourceCopy)
      ? overallInputCopy
      : undefined}
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
        definition={experiment.assessments.find(
          item => item.id === assessment.id
        )}
        showScore={mode === 'results'}
        showHeading={assessment.id !== overall?.id}
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
{#snippet telemetrySection()}
  <EvalTelemetrySection
    entries={references}
    nodes={telemetryNodes}
    loading={telemetryLoading}
    wholeTraces={row.kind === 'result' || row.kind === 'case'}
    evidenceLogs={span &&
    (row.kind === 'tool' ||
      row.kind === 'span' ||
      row.kind === 'grader' ||
      row.kind === 'execution')
      ? evidenceLogs
      : []}
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
