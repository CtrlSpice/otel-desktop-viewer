<script lang="ts">
  import Identifier from '@/components/shared/Identifier.svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import type { EvalRun, EvalAttempt, TelemetryReference } from './model'
  import { cost, estimatedCost, localTimestamp, traceHref } from './model'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import { completionLabel } from './group-attempts'

  let {
    run,
    attempt,
  }: {
    run: EvalRun
    attempt?: EvalAttempt
  } = $props()
  const investigator = $derived(estimatedCost(run.attempts, 'investigatorCost'))
  const grader = $derived(estimatedCost(run.attempts, 'graderCost'))
  const startedAt = $derived(
    attempt?.startedAt ?? (!attempt ? run.startedAt : undefined)
  )
</script>

{#snippet telemetry(reference: TelemetryReference)}
  <DetailField
    fieldName="traceId"
    fieldType="bytes"
    fieldValue={reference.traceID}
  >
    {#snippet value()}<a
        class="detail-cell__value link link-primary"
        href={traceHref(reference)}><Identifier value={reference.traceID} /></a
      >{/snippet}
  </DetailField>
  {#if reference.spanID}{@const spanID = reference.spanID}<DetailField
      fieldName="spanId"
      fieldType="bytes"
      fieldValue={reference.spanID}
    >
      {#snippet value()}<a
          class="detail-cell__value link link-primary"
          href={traceHref(reference)}><Identifier value={spanID} /></a
        >{/snippet}
    </DetailField>{/if}
{/snippet}

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
          />
          <DetailField
            fieldName="evaluation"
            fieldType="string"
            fieldValue={run.evaluation}
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
    <DetailBubble label="Question">
      {#snippet action()}<ValueCopyButton
          text={attempt.question}
          label="question"
        />{/snippet}
      <p class="eval-detail__prose">{attempt.question}</p>
    </DetailBubble>
    <DetailBubble
      label={`Attempt ${attempt.repetition} · ${completionLabel(attempt.state, true)}`}
      initialOpen={false}
    >
      {#snippet action()}
        {#if attempt.answer !== undefined}<ValueCopyButton
            text={attempt.answer}
            label="answer"
          />{/if}
      {/snippet}
      <p class="eval-detail__prose">{attempt.answer ?? 'No answer recorded'}</p>
      {#if attempt.error}
        <table class="detail-fields w-full" aria-label="Attempt fault">
          <tbody
            ><DetailField
              fieldName="internal fault"
              fieldType="string"
              showType={false}
              fieldValue={attempt.error}
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
                showType={false}
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
              showType={false}
              fieldValue={attempt.seconds === undefined
                ? 'Unknown'
                : `${attempt.seconds} s`}
            />
            <DetailField
              fieldName="investigator estimate (CAD)"
              fieldType="string"
              showType={false}
              fieldValue={cost(attempt.investigatorCost)}
            />
            <DetailField
              fieldName="grader estimate (CAD)"
              fieldType="string"
              showType={false}
              fieldValue={cost(attempt.graderCost)}
            />
          </tbody>
        </table>
        {#if attempt.telemetry}<FieldGroup label="Telemetry" detail
            ><table class="detail-fields w-full" aria-label="Attempt telemetry">
              <tbody>{@render telemetry(attempt.telemetry)}</tbody>
            </table></FieldGroup
          >{/if}
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
                  showType={false}
                  fieldValue={assessment.explanation}
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
                    showType={false}
                    fieldValue={`${assessment.value ?? 'Unscored'} / ${definition.maximum}`}
                  />
                  <DetailField
                    fieldName="evaluator"
                    fieldType="string"
                    showType={false}
                    fieldValue={assessment.evaluator}
                  />
                  <DetailField
                    fieldName="rubric"
                    fieldType="string"
                    showType={false}
                    fieldValue={assessment.rubric}
                  />
                  {#if assessment.telemetry}{@render telemetry(
                      assessment.telemetry
                    )}{/if}
                {:else}<DetailField
                    fieldName="score"
                    fieldType="string"
                    showType={false}
                    fieldValue="Unscored"
                  />{/if}
              </tbody>
            </table>
          </FieldGroup>
        </FieldGroup>
      {/each}
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
