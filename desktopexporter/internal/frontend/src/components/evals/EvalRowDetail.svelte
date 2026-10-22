<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import EvalAssessmentDetail from './EvalAssessmentDetail.svelte'
  import MarkdownText from '@/components/shared/MarkdownText.svelte'
  import { cost, localTimestamp, type EvalRun, type EvalView } from './model'
  import { caseSummary } from './case-summary'
  import { runSummary } from './run-summary'
  import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
  import { evalTelemetrySummaries } from './eval-telemetry-summaries'
  import { referenceKey } from './eval-traces'
  import type { SpanNode } from '@/types/api-types'
  import { completionLabel, completionTone } from './group-reps'
  import type { EvalOutlineRow } from './eval-outline'
  import { formatDuration } from '@/utils/time'
  import type { EvalSourceFieldCopy } from './eval-source-copy'

  let {
    run,
    row,
    mode = 'results',
    telemetryNodes,
    telemetryLoading = false,
    sourceCopy,
  }: {
    run: EvalRun
    row: EvalOutlineRow
    mode?: EvalView
    telemetryNodes?: Map<string, SpanNode>
    telemetryLoading?: boolean
    sourceCopy?: EvalSourceFieldCopy
  } = $props()
  const rep = $derived(row.rep)
  const references = $derived(evalTelemetrySummaries(run, row))
  const members = $derived(run.reps.filter(item => item.caseID === row.caseID))
  const summary = $derived(caseSummary(members, run.assessments))
  const outcomes = $derived(runSummary({ ...run, reps: members }).parts)
  const tool = $derived(
    rep?.tools.find((_, index) => row.id === `${rep.id}/tool/${index}`)
  )
  const assessments = $derived(
    rep?.assessments.filter(item =>
      row.kind === 'rep' || row.kind === 'case'
        ? mode === 'results'
        : row.kind === 'grading' || item.id === row.assessmentID
    ) ?? []
  )
  const prose = $derived(
    mode === 'cost'
      ? undefined
      : row.kind === 'case' && !rep
        ? mode === 'results'
          ? members[0]?.question
          : undefined
        : row.kind === 'rep' || row.kind === 'case'
          ? mode === 'results'
            ? rep?.answer
            : undefined
          : row.kind === 'execution'
            ? rep?.answer
            : tool?.message
  )
  const overallDefinition = $derived(
    run.assessments.find(
      item => item.id === run.headlineAssessment && item.name === 'Overall'
    )
  )
  const overall = $derived(
    mode === 'results'
      ? assessments.find(item => item.id === overallDefinition?.id)
      : undefined
  )
  const proseCopy = $derived(
    row.kind === 'case' && !rep && members[0]
      ? sourceCopy?.(members[0].id, 'question')
      : (row.kind === 'case' ||
            row.kind === 'rep' ||
            row.kind === 'execution') &&
          rep
        ? sourceCopy?.(rep.id, 'answer')
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
    row.kind === 'case' || row.kind === 'rep'
      ? [
          {
            name: 'investigator estimate (CAD)',
            estimate:
              row.kind === 'case'
                ? summary.investigatorCost
                : {
                    total: rep?.investigatorCost ?? null,
                    incomplete: false,
                  },
          },
          {
            name: 'grader estimate (CAD)',
            estimate:
              row.kind === 'case'
                ? summary.graderCost
                : { total: rep?.graderCost ?? null, incomplete: false },
          },
          { name: 'total estimate (CAD)', estimate: row.estimate },
        ]
      : [{ name: 'estimate (CAD)', estimate: row.estimate }]
  )
  const calls = $derived(
    row.kind === 'case' ? summary.calls : (rep?.tools.length ?? 0)
  )
  const failures = $derived(
    row.kind === 'case'
      ? summary.failures
      : (rep?.tools.filter(item => item.state === 'Error').length ?? 0)
  )
</script>

<DetailStack
  label={row.kind === 'rep' ? 'Result details' : `${row.name} details`}
  storageKey={`eval-${row.kind}-detail-layout`}
>
  <DetailBubble
    label={row.name}
    layoutKey={row.kind === 'case' ? 'Question' : row.name}
  >
    {#snippet trailing()}<EvalStatus
        label={completionLabel(row.state, row.internalFault)}
        tone={completionTone(row.state, row.internalFault)}
      />{/snippet}
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
      {#if row.kind === 'case' && rep && mode === 'results'}
        <table class="detail-fields w-full" aria-label="Test case input">
          <tbody>
            <DetailField
              fieldName="input"
              fieldType="string"
              fieldValue={rep.question}
              loadText={sourceCopy?.(rep.id, 'question')}
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
          {@const fieldName = row.kind === 'case' && !rep ? 'input' : 'output'}
          <table
            class="detail-fields w-full"
            aria-label={`${row.name} ${fieldName}`}
          >
            <tbody>
              <DetailField
                {fieldName}
                fieldType={fieldName === 'output'
                  ? (rep?.answerType ?? 'string')
                  : 'string'}
                fieldValue={prose}
                loadText={proseCopy}
                multiline
              >
                {#snippet value()}
                  {#if fieldName === 'output' && rep?.answerType && rep.answerType !== 'string'}
                    {prose}
                  {:else}
                    <MarkdownText text={prose === '' ? '""' : prose} />
                  {/if}
                {/snippet}
              </DetailField>
            </tbody>
          </table>
        {/if}
      {:else if row.kind === 'rep' && mode === 'results'}
        <p class="eval-row-detail__prose">No answer recorded</p>
      {/if}

      {#if mode === 'results' && row.kind === 'case' && !rep}
        <div
          class="eval-row-detail__outcomes"
          role="list"
          aria-label="Test case outcomes"
        >
          {#each outcomes as outcome}
            <span class="inline-flex items-center gap-2" role="listitem">
              <EvalStatus label={outcome.label} tone={outcome.tone} />
              <span class="text-rp-subtle tabular-nums">[{outcome.count}]</span>
            </span>
          {/each}
        </div>
      {/if}

      {#if (row.kind === 'case' || row.kind === 'rep' || row.kind === 'execution') && rep?.error !== undefined}
        <table class="detail-fields w-full" aria-label="Result fault">
          <tbody
            ><DetailField
              fieldName="internal fault"
              fieldType="string"
              fieldValue={rep.error}
              loadText={sourceCopy?.(rep.id, 'error')}
              multiline
            /></tbody
          >
        </table>
      {/if}
    {/if}

    <table class="detail-fields w-full" aria-label={`${row.name} fields`}>
      <tbody>
        {#if (row.kind === 'rep' || row.kind === 'case') && rep?.sourceID}
          <DetailField
            fieldName="result ID"
            fieldType="string"
            fieldValue={rep.sourceID}
            identifier
          />
        {/if}
        {#if row.kind === 'case' && !rep}
          <DetailField
            fieldName="results"
            fieldType="number"
            fieldValue={String(members.length)}
          />
        {/if}
        {#if mode === 'time' && ['case', 'rep', 'execution'].includes(row.kind)}
          <DetailField
            fieldName="tool calls"
            fieldType="number"
            fieldValue={members.some(item => item.toolsKnown === false)
              ? 'Unknown'
              : `${calls} total · ${failures} failed`}
          />
        {/if}
        {#if mode !== 'cost'}
          {#if (row.kind === 'case' || row.kind === 'rep') && rep?.startedAt !== undefined}
            <DetailField
              fieldName="started"
              fieldType="timestamp"
              fieldValue={rep.startedAt}
              timestampLabel={localTimestamp(rep.startedAt)}
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
  {#if mode !== 'cost' && assessments.length}
    <DetailBubble label="Assessments">
      {#snippet trailing()}
        {#if overall}
          <span class="inline-flex items-center gap-2">
            {#if overall.rubric || sourceCopy}
              <ValueCopyButton
                text={overall.rubric}
                loadText={rep
                  ? sourceCopy?.(rep.id, `assessment:${overall.id}:rubric`)
                  : undefined}
                label="grader input"
              />
            {/if}
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
          definition={run.assessments.find(item => item.id === assessment.id)}
          showScore={mode === 'results'}
          showHeading={assessment.id !== overall?.id}
          loadRubric={rep
            ? sourceCopy?.(rep.id, `assessment:${assessment.id}:rubric`)
            : undefined}
          loadExplanation={rep
            ? sourceCopy?.(
                rep.id,
                assessment.id === run.headlineAssessment
                  ? 'explanation'
                  : `assessment:${assessment.id}:explanation`
              )
            : undefined}
        />
      {/each}
    </DetailBubble>
  {/if}
  <DetailBubble
    label="Telemetry"
    disabled={references.length === 0}
    initialOpen={references.length > 0}
  >
    {#if references.length}
      <EvalTelemetrySummaries
        entries={references}
        nodes={telemetryNodes}
        loading={telemetryLoading}
      />
    {/if}
  </DetailBubble>
</DetailStack>

<style lang="postcss">
  @reference "../../app.css";
  .eval-row-detail__prose {
    @apply py-2 text-sm text-base-content;
    padding-inline: var(--pane-inline-inset);
    overflow-wrap: anywhere;
  }
  .eval-row-detail__outcomes {
    @apply flex flex-wrap items-center gap-x-4 text-sm;
    padding-inline: var(--pane-inline-inset);
  }
</style>
