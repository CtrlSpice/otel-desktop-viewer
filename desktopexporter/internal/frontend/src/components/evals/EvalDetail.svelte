<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import type { EvalRun, EvalAttempt } from './model'
  import { cost, estimatedCost, localTimestamp } from './model'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import { completionLabel, completionTone } from './group-attempts'
  import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
  import { evalTelemetrySummaries } from './eval-telemetry-summaries'
  import type { SpanNode } from '@/types/api-types'
  import type { SnapshotFieldCopy, SnapshotRunCopy } from './snapshot-copy'

  let {
    run,
    attempt,
    telemetryNodes,
    telemetryLoading = false,
    sourceCopy,
    runCopy,
  }: {
    run: EvalRun
    attempt?: EvalAttempt
    telemetryNodes?: Map<string, SpanNode>
    telemetryLoading?: boolean
    sourceCopy?: SnapshotFieldCopy
    runCopy?: SnapshotRunCopy
  } = $props()
  const investigator = $derived(estimatedCost(run.attempts, 'investigatorCost'))
  const references = $derived(evalTelemetrySummaries(run, undefined, attempt))
  const grader = $derived(estimatedCost(run.attempts, 'graderCost'))
  const startedAt = $derived(
    attempt?.startedAt ?? (!attempt ? run.startedAt : undefined)
  )
</script>

<DetailStack
  label={attempt ? 'Attempt details' : 'Run details'}
  storageKey={attempt ? 'eval-attempt-detail-layout' : 'eval-run-detail-layout'}
>
  {#if !attempt}
    <DetailBubble label="Run">
      <table class="detail-fields w-full" aria-label="Run fields">
        <tbody>
          <DetailField
            fieldName="name"
            fieldType="string"
            fieldValue={run.name}
            loadText={runCopy?.(run.id, 'name')}
          />
          <DetailField
            fieldName="evaluation"
            fieldType="string"
            fieldValue={run.evaluation}
            loadText={runCopy?.(run.id, 'evaluation')}
          />
          <DetailField
            fieldName="state"
            fieldType="string"
            fieldValue={run.state}
            >{#snippet value()}<RunStatusBadge
                state={run.state}
                pauseReason={run.pauseReason}
              />{/snippet}</DetailField
          >
          <DetailField
            fieldName="source"
            fieldType="string"
            fieldValue={run.source}
            loadText={runCopy?.(run.id, 'source')}
          />
          {#if startedAt}<DetailField
              fieldName="started"
              fieldType="timestamp"
              fieldValue={startedAt}
              timestampLabel={localTimestamp(startedAt)}
              >{#snippet value()}<time
                  class="text-sm text-base-content tabular-nums"
                  datetime={startedAt}
                  title={startedAt}>{localTimestamp(startedAt)}</time
                >{/snippet}</DetailField
            >{/if}
        </tbody>
      </table>
    </DetailBubble>
    <DetailBubble
      label="Configuration"
      initialOpen={false}
      count={run.configuration.length}
      countLabel="fields"
    >
      <table class="detail-fields w-full" aria-label="Run configuration">
        <tbody
          >{#each run.configuration as field}<DetailField
              fieldName={field.name}
              fieldType="string"
              fieldValue={field.value}
            />{/each}</tbody
        >
      </table>
    </DetailBubble>
    <DetailBubble label="Grading" initialOpen={false}>
      <table class="detail-fields w-full" aria-label="Grading for this run">
        <tbody>
          <DetailField
            fieldName="status"
            fieldType="string"
            fieldValue="Provisional"
          />
          <DetailField
            fieldName="assessed attempts"
            fieldType="string"
            fieldValue={`${run.attempts.filter(item => item.assessments.length > 0).length} / ${run.attempts.length}`}
          />
          <DetailField
            fieldName="grader estimate (CAD)"
            fieldType="string"
            fieldValue={`${cost(grader.total)}${grader.incomplete && grader.total !== null ? ' · incomplete' : ''}`}
          />
          <DetailField
            fieldName="investigator estimate (CAD)"
            fieldType="string"
            fieldValue={`${cost(investigator.total)}${investigator.incomplete && investigator.total !== null ? ' · incomplete' : ''}`}
          />
        </tbody>
      </table>
    </DetailBubble>
  {:else}
    <DetailBubble label={`Attempt ${attempt.repetition}`}>
      {#snippet trailing()}<EvalStatus
          label={completionLabel(attempt.state, true)}
          tone={completionTone(attempt.state, true)}
        />{/snippet}
      {#snippet action()}
        {#if attempt.answer !== undefined}<ValueCopyButton
            text={attempt.answer}
            loadText={sourceCopy?.(attempt.id, 'answer')}
            label="answer"
          />{/if}
      {/snippet}
      <p class="eval-detail__prose">{attempt.answer ?? 'No answer recorded'}</p>
      {#if attempt.error !== undefined}
        <table class="detail-fields w-full" aria-label="Attempt fault">
          <tbody
            ><DetailField
              fieldName="internal fault"
              fieldType="string"
              fieldValue={attempt.error}
              loadText={sourceCopy?.(attempt.id, 'error')}
              multiline
            >
              {#snippet value()}<span
                  class="text-base-content whitespace-pre-wrap"
                  >{attempt.error}</span
                >{/snippet}
            </DetailField></tbody
          >
        </table>
      {/if}
      <FieldGroup label="Execution details" detail open={false}>
        <table class="detail-fields w-full" aria-label="Attempt fields">
          <tbody>
            {#if startedAt}<DetailField
                fieldName="started"
                fieldType="timestamp"
                fieldValue={startedAt}
                timestampLabel={localTimestamp(startedAt)}
                >{#snippet value()}<time
                    class="text-sm text-base-content tabular-nums"
                    datetime={startedAt}
                    title={startedAt}>{localTimestamp(startedAt)}</time
                  >{/snippet}</DetailField
              >{/if}
          </tbody>
        </table>
        <table class="detail-fields w-full" aria-label="Attempt accounting">
          <tbody>
            <DetailField
              fieldName="duration"
              fieldType="string"
              fieldValue={attempt.seconds === undefined
                ? 'Unknown'
                : `${attempt.seconds} s`}
            />
            <DetailField
              fieldName="investigator estimate (CAD)"
              fieldType="string"
              fieldValue={cost(attempt.investigatorCost)}
            />
            <DetailField
              fieldName="grader estimate (CAD)"
              fieldType="string"
              fieldValue={cost(attempt.graderCost)}
            />
          </tbody>
        </table>
      </FieldGroup>
    </DetailBubble>
    <DetailBubble
      label="Assessments"
      count={attempt.assessments.length}
      initialOpen={false}
    >
      {#each run.assessments as definition}
        {@const assessment = attempt.assessments.find(
          item => item.id === definition.id
        )}
        <FieldGroup label={definition.name} detail>
          {#snippet heading()}<span class="font-normal">{definition.name}</span
            ><AssessmentBadge
              {assessment}
              maximum={definition.maximum}
            />{/snippet}
          {#if assessment}
            <table
              class="detail-fields w-full"
              aria-label={`${definition.name} explanation`}
            >
              <tbody
                ><DetailField
                  fieldName="explanation"
                  fieldType="string"
                  fieldValue={assessment.explanation}
                  loadText={sourceCopy?.(attempt.id, 'explanation')}
                  multiline
                /></tbody
              >
            </table>
          {/if}
          <FieldGroup label="Grading details" detail open={false}>
            <table class="detail-fields w-full" aria-label={definition.name}>
              <tbody>
                {#if assessment}
                  <DetailField
                    fieldName="score"
                    fieldType="string"
                    fieldValue={`${assessment.value ?? 'Unscored'} / ${definition.maximum}`}
                  />
                  <DetailField
                    fieldName="evaluator"
                    fieldType="string"
                    fieldValue={assessment.evaluator}
                  />
                  <DetailField
                    fieldName="rubric"
                    fieldType="string"
                    fieldValue={assessment.rubric}
                  />
                {:else}<DetailField
                    fieldName="score"
                    fieldType="string"
                    fieldValue="Unscored"
                  />{/if}
              </tbody>
            </table>
            {#if assessment?.telemetry}
              <EvalTelemetrySummaries
                entries={[
                  { reference: assessment.telemetry, label: definition.name },
                ]}
                nodes={telemetryNodes}
                loading={telemetryLoading}
              />
            {/if}
          </FieldGroup>
        </FieldGroup>
      {/each}
    </DetailBubble>
  {/if}
  {#if references.length}
    <DetailBubble
      label="Telemetry"
      count={references.length}
      countLabel="references"
      initialOpen={false}
    >
      <EvalTelemetrySummaries
        entries={references}
        nodes={telemetryNodes}
        loading={telemetryLoading}
      />
    </DetailBubble>
  {/if}
</DetailStack>

<style lang="postcss">
  @reference "../../app.css";

  .eval-detail__prose {
    @apply py-2 text-sm text-base-content whitespace-pre-wrap;
    padding-inline: var(--pane-inline-inset);
    overflow-wrap: anywhere;
  }
</style>
