<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import { cost, type EvalRun, type EvalView } from './model'
  import { caseSummary } from './case-summary'
  import { runSummary } from './run-summary'
  import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
  import { evalTelemetrySummaries } from './eval-telemetry-summaries'
  import { referenceKey } from './eval-traces'
  import type { SpanNode } from '@/types/api-types'
  import { completionLabel, completionTone } from './group-attempts'
  import type { EvalOutlineRow } from './eval-outline'
  import { formatDuration } from '@/utils/time'
  import type { SnapshotFieldCopy } from './snapshot-copy'

  let {
    run,
    row,
    mode = 'results',
    parentName,
    telemetryNodes,
    telemetryLoading = false,
    sourceCopy,
  }: {
    run: EvalRun
    row: EvalOutlineRow
    mode?: EvalView
    parentName?: string
    telemetryNodes?: Map<string, SpanNode>
    telemetryLoading?: boolean
    sourceCopy?: SnapshotFieldCopy
  } = $props()
  const attempt = $derived(row.attempt)
  const references = $derived(evalTelemetrySummaries(run, row))
  const members = $derived(
    run.attempts.filter(item => item.caseID === row.caseID)
  )
  const summary = $derived(caseSummary(members, run.assessments))
  const outcomes = $derived(runSummary({ ...run, attempts: members }).parts)
  const tool = $derived(
    attempt?.tools.find((_, index) => row.id === `${attempt.id}/tool/${index}`)
  )
  const assessments = $derived(
    attempt?.assessments.filter(item =>
      row.kind === 'attempt'
        ? mode === 'results'
        : row.kind === 'grading' || item.id === row.assessmentID
    ) ?? []
  )
  const prose = $derived(
    mode === 'cost'
      ? undefined
      : row.kind === 'case'
        ? mode === 'results'
          ? members[0]?.question
          : undefined
        : row.kind === 'attempt'
          ? mode === 'results'
            ? attempt?.answer
            : undefined
          : row.kind === 'execution'
            ? attempt?.answer
            : tool?.message
  )
  const proseCopy = $derived(
    row.kind === 'case' && members[0]
      ? sourceCopy?.(members[0].id, 'question')
      : (row.kind === 'attempt' || row.kind === 'execution') && attempt
        ? sourceCopy?.(attempt.id, 'answer')
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
        : row.kind === 'case' && summary.duration.total !== null
          ? `${summary.duration.total} s${summary.duration.incomplete ? ' · partial total' : ''}`
          : 'Unknown'
  )
  const durationSource = $derived(
    row.window
      ? span && span.endTime >= span.startTime
        ? 'recorded span'
        : 'available reference extent'
      : row.seconds !== undefined
        ? 'recorded duration'
        : row.kind === 'case' && summary.duration.total !== null
          ? 'sum of recorded durations'
          : undefined
  )
  const costFields = $derived(
    row.kind === 'case' || row.kind === 'attempt'
      ? [
          {
            name: 'investigator estimate (CAD)',
            estimate:
              row.kind === 'case'
                ? summary.investigatorCost
                : {
                    total: attempt?.investigatorCost ?? null,
                    incomplete: false,
                  },
          },
          {
            name: 'grader estimate (CAD)',
            estimate:
              row.kind === 'case'
                ? summary.graderCost
                : { total: attempt?.graderCost ?? null, incomplete: false },
          },
          { name: 'total estimate (CAD)', estimate: row.estimate },
        ]
      : [{ name: 'estimate (CAD)', estimate: row.estimate }]
  )
  const calls = $derived(
    row.kind === 'case' ? summary.calls : (attempt?.tools.length ?? 0)
  )
  const failures = $derived(
    row.kind === 'case'
      ? summary.failures
      : (attempt?.tools.filter(item => item.state === 'Error').length ?? 0)
  )
</script>

<DetailStack
  label={row.kind === 'attempt' ? 'Attempt details' : `${row.name} details`}
  storageKey={`eval-${row.kind}-detail-layout`}
>
  <DetailBubble
    label="Details"
    layoutKey={row.kind === 'case' ? 'Question' : row.name}
  >
    {#snippet trailing()}<EvalStatus
        label={completionLabel(row.state, row.internalFault)}
        tone={completionTone(row.state, row.internalFault)}
      />{/snippet}
    {#snippet action()}
      {#if prose !== undefined}<ValueCopyButton
          text={prose}
          loadText={proseCopy}
          label={row.kind === 'attempt' ? 'answer' : row.name}
        />{/if}
    {/snippet}
    <p class="eval-row-detail__context">
      <span class="text-base-content">{row.name}</span>
      {#if parentName}<span>{' · '}{parentName}</span>{/if}
    </p>

    {#if mode === 'cost'}
      <table class="detail-fields w-full" aria-label={`${row.name} accounting`}>
        <tbody>
          {#each costFields as field}
            <DetailField
              fieldName={field.name}
              fieldType="string"
              fieldValue={`${cost(field.estimate.total)}${field.estimate.incomplete && field.estimate.total !== null ? ' · partial' : ''}`}
            />
          {/each}
        </tbody>
      </table>
    {:else}
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
        {:else}<p class="eval-row-detail__prose">
            {prose === '' ? '""' : prose}
          </p>{/if}
      {:else if row.kind === 'attempt' && mode === 'results'}
        <p class="eval-row-detail__prose">No answer recorded</p>
      {/if}

      {#if mode === 'results' && row.kind === 'case'}
        <div
          class="eval-row-detail__outcomes"
          role="list"
          aria-label="Case outcomes"
        >
          {#each outcomes as outcome}
            <span class="inline-flex items-center gap-2" role="listitem">
              <EvalStatus label={outcome.label} tone={outcome.tone} />
              <span class="text-rp-subtle tabular-nums">[{outcome.count}]</span>
            </span>
          {/each}
        </div>
      {/if}

      {#if (row.kind === 'attempt' || row.kind === 'execution') && attempt?.error !== undefined}
        <table class="detail-fields w-full" aria-label="Attempt fault">
          <tbody
            ><DetailField
              fieldName="internal fault"
              fieldType="string"
              fieldValue={attempt.error}
              loadText={sourceCopy?.(attempt.id, 'error')}
              multiline
            /></tbody
          >
        </table>
      {/if}

      {#each assessments as assessment}
        {@const definition = run.assessments.find(
          item => item.id === assessment.id
        )}
        <FieldGroup label={definition?.name ?? assessment.id} detail>
          {#snippet heading()}
            <span>{definition?.name ?? assessment.id}</span>
            {#if mode === 'results'}<AssessmentBadge
                {assessment}
                maximum={definition?.maximum}
              />{/if}
          {/snippet}
          <table
            class="detail-fields w-full"
            aria-label={`${definition?.name ?? assessment.id} explanation`}
          >
            <tbody>
              {#if mode === 'results'}
                <DetailField
                  fieldName="score"
                  fieldType="string"
                  fieldValue={assessment.value === null
                    ? 'Unscored'
                    : `${assessment.value}${definition ? ` / ${definition.maximum}` : ''}`}
                />
                <DetailField
                  fieldName="explanation"
                  fieldType="string"
                  fieldValue={assessment.explanation}
                  loadText={attempt
                    ? sourceCopy?.(attempt.id, 'explanation')
                    : undefined}
                  multiline
                />
              {/if}
              <DetailField
                fieldName="evaluator"
                fieldType="string"
                fieldValue={assessment.evaluator}
              />
              <DetailField
                fieldName="rubric"
                fieldType="string"
                fieldValue={assessment.rubric}
                multiline
              />
            </tbody>
          </table>
        </FieldGroup>
      {/each}
    {/if}

    <table class="detail-fields w-full" aria-label={`${row.name} fields`}>
      <tbody>
        {#if row.kind === 'case'}
          <DetailField
            fieldName="attempts"
            fieldType="string"
            fieldValue={String(members.length)}
          />
        {/if}
        {#if mode === 'time' && ['case', 'attempt', 'execution'].includes(row.kind)}
          <DetailField
            fieldName="tool calls"
            fieldType="string"
            fieldValue={`${calls} total · ${failures} failed`}
          />
        {/if}
        {#if mode !== 'cost'}
          <DetailField
            fieldName="duration"
            fieldType="string"
            fieldValue={duration}
            origin={durationSource}
          />
        {/if}
        {#if row.kind !== 'case' && mode !== 'time'}
          <DetailField
            fieldName="case"
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
  <DetailBubble label="Telemetry" initialOpen={references.length > 0}>
    {#if references.length}
      <EvalTelemetrySummaries
        entries={references}
        nodes={telemetryNodes}
        loading={telemetryLoading}
      />
    {:else}<p class="eval-row-detail__context">
        No telemetry reference recorded for this row.
      </p>{/if}
  </DetailBubble>
</DetailStack>

<style lang="postcss">
  @reference "../../app.css";
  .eval-row-detail__prose {
    @apply py-2 text-sm text-base-content whitespace-pre-wrap;
    padding-inline: var(--pane-inline-inset);
    overflow-wrap: anywhere;
  }
  .eval-row-detail__context {
    @apply py-2 text-sm;
    color: var(--color-subtle);
    padding-inline: var(--pane-inline-inset);
    overflow-wrap: anywhere;
  }
  .eval-row-detail__outcomes {
    @apply flex flex-wrap items-center gap-x-4 text-sm;
    padding-inline: var(--pane-inline-inset);
  }
</style>
