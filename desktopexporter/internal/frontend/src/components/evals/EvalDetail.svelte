<script lang="ts">
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import DetailField from '@/components/shared/DetailField.svelte'
  import type { EvalRun, EvalAttempt, TelemetryReference } from './model'
  import { cost, estimatedCost, localTimestamp, traceHref } from './model'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import RunStatusBadge from './RunStatusBadge.svelte'

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
  <DetailField fieldName="trace id" fieldType="string">
    {#snippet value()}<a
        class="detail-cell__value link link-primary font-mono"
        href={traceHref(reference)}>{reference.traceID}</a
      >{/snippet}
  </DetailField>
  {#if reference.spanID}<DetailField fieldName="span id" fieldType="string">
      {#snippet value()}<a
          class="detail-cell__value link link-primary font-mono"
          href={traceHref(reference)}>{reference.spanID}</a
        >{/snippet}
    </DetailField>{/if}
{/snippet}

{#if !attempt}<PaneHeader
    mode="title"
    title="Run details"
    ariaLabel="Run detail heading"
  />{/if}

<DetailStack label={attempt ? 'Attempt details' : 'Run details'}>
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
          <DetailField fieldName="state" fieldType="string"
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
          {#if startedAt}<DetailField fieldName="started" fieldType="timestamp"
              >{#snippet value()}<time
                  class="text-sm text-rp-subtle tabular-nums"
                  datetime={startedAt}
                  title={startedAt}>{localTimestamp(startedAt)}</time
                >{/snippet}</DetailField
            >{/if}
        </tbody>
      </table>
    </DetailBubble>
    <DetailBubble
      label="Configuration"
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
    <DetailBubble label="Grading">
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
    <DetailBubble label="Fields">
      <FieldGroup label="Attempt" detail>
        <table class="detail-fields w-full" aria-label="Attempt fields">
          <tbody>
            <DetailField
              fieldName="name"
              fieldType="string"
              fieldValue={attempt.name}
            />
            <DetailField fieldName="state" fieldType="string"
              >{#snippet value()}<span
                  class:text-papaya={attempt.state === 'Error'}
                  >{attempt.state}</span
                >{/snippet}</DetailField
            >
            <DetailField
              fieldName="repetition"
              fieldType="integer"
              fieldValue={String(attempt.repetition)}
            />
            {#if startedAt}<DetailField
                fieldName="started"
                fieldType="timestamp"
                >{#snippet value()}<time
                    class="text-sm text-rp-subtle tabular-nums"
                    datetime={startedAt}
                    title={startedAt}>{localTimestamp(startedAt)}</time
                  >{/snippet}</DetailField
              >{/if}
            {#if attempt.error}<DetailField
                fieldName="execution error"
                fieldType="string"
                multiline
                >{#snippet value()}<span class="text-papaya whitespace-pre-wrap"
                    >{attempt.error}</span
                  >{/snippet}</DetailField
              >{/if}
            <DetailField
              fieldName="question"
              fieldType="string"
              fieldValue={attempt.question}
              multiline
            />
            <DetailField
              fieldName="answer"
              fieldType="string"
              fieldValue={attempt.answer ?? 'No answer recorded'}
              multiline
            />
          </tbody>
        </table>
      </FieldGroup>
      <FieldGroup label="Accounting" detail>
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
      {#if attempt.telemetry}<FieldGroup label="Telemetry" detail
          ><table class="detail-fields w-full" aria-label="Attempt telemetry">
            <tbody>{@render telemetry(attempt.telemetry)}</tbody>
          </table></FieldGroup
        >{/if}
    </DetailBubble>
    <DetailBubble label="Assessments" count={attempt.assessments.length}>
      {#each run.assessments as definition}
        {@const assessment = attempt.assessments.find(
          item => item.id === definition.id
        )}
        <FieldGroup label={definition.name} detail>
          {#snippet heading()}<span>{definition.name}</span><AssessmentBadge
              {assessment}
              maximum={definition.maximum}
            />{/snippet}
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
                <DetailField
                  fieldName="explanation"
                  fieldType="string"
                  fieldValue={assessment.explanation}
                  multiline
                />
                {#if assessment.telemetry}{@render telemetry(
                    assessment.telemetry
                  )}{/if}
              {:else}<DetailField
                  fieldName="score"
                  fieldType="string"
                  fieldValue="Unscored"
                />{/if}
            </tbody>
          </table>
        </FieldGroup>
      {/each}
    </DetailBubble>
    <DetailBubble label="Tool calls" count={attempt.tools.length}>
      {#each attempt.tools as tool, index}
        <FieldGroup label={`${tool.name} ${index + 1}`} detail open={false}>
          {#snippet heading()}<span>{tool.name}</span
            >{#if tool.state === 'Error'}<span
                class="badge badge-xs badge-soft badge-error">Error</span
              >{/if}{/snippet}
          <table
            class="detail-fields w-full"
            aria-label={`${tool.name} ${index + 1}`}
          >
            <tbody>
              <DetailField
                fieldName="output"
                fieldType="string"
                fieldValue={tool.message}
                multiline
              />
              {#if tool.telemetry}{@render telemetry(tool.telemetry)}{/if}
            </tbody>
          </table>
        </FieldGroup>
      {/each}
      {#if !attempt.tools.length}<p class="p-4 text-sm">
          No tool calls recorded
        </p>{/if}
    </DetailBubble>
  {/if}
</DetailStack>
