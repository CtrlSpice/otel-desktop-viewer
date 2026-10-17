<script lang="ts">
  import PaneHeader, { paneTabID } from '@/components/shared/PaneHeader.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import DetailField from '@/components/shared/DetailField.svelte'
  import type { EvalRun, EvalAttempt, TelemetryReference } from './model'
  import { cost, estimatedCost, localTimestamp, traceHref } from './model'
  import AssessmentBadge from './AssessmentBadge.svelte'

  let {
    run,
    attempt,
    tab,
    onTab,
  }: {
    run: EvalRun
    attempt?: EvalAttempt
    tab: string
    onTab: (tab: string) => void
  } = $props()
  const investigator = $derived(estimatedCost(run.attempts, 'investigatorCost'))
  const grader = $derived(estimatedCost(run.attempts, 'graderCost'))
  const startedAt = $derived(
    attempt?.startedAt ?? (!attempt ? run.startedAt : undefined)
  )
  const panelID = 'eval-detail-tabpanel'
  const tabs = $derived([
    { id: 'Overview', label: 'Fields' },
    {
      id: 'Assessments',
      label: 'Assessments',
      count: attempt?.assessments.length ?? 0,
    },
    {
      id: 'Tool calls',
      label: 'Tool calls',
      count: attempt?.tools.length ?? 0,
    },
  ])
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

{#if attempt}
  <PaneHeader
    mode="tabs"
    {tabs}
    activeID={tab}
    onSelect={onTab}
    tabPanelID={panelID}
    ariaLabel="Attempt details"
  />
{:else}<PaneHeader mode="title" title="Run details" />{/if}

<!-- svelte-ignore a11y_no_noninteractive_tabindex (The scrollable inspector must be keyboard accessible when it contains no controls.) -->
<div
  class="min-h-0 flex-1 overflow-y-auto"
  id={panelID}
  role={attempt ? 'tabpanel' : 'region'}
  tabindex="0"
  aria-labelledby={attempt ? paneTabID(panelID, tab) : undefined}
  aria-label={attempt ? undefined : 'Run details'}
>
  {#if !attempt}
    <FieldGroup label="Run" detail>
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
          />
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
    </FieldGroup>
    <FieldGroup label="Configuration" count={run.configuration.length} detail>
      <table class="detail-fields w-full" aria-label="Run configuration">
        <tbody
          >{#each run.configuration as field}<DetailField
              fieldName={field.name}
              fieldType="string"
              fieldValue={field.value}
            />{/each}</tbody
        >
      </table>
    </FieldGroup>
    <FieldGroup label="Grading" detail>
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
    </FieldGroup>
  {:else if tab === 'Overview'}
    <FieldGroup label="Attempt" detail>
      <table class="detail-fields w-full" aria-label="Attempt fields">
        <tbody>
          <DetailField
            fieldName="name"
            fieldType="string"
            fieldValue={attempt.name}
          />
          <DetailField
            fieldName="state"
            fieldType="string"
            fieldValue={attempt.state}
          />
          <DetailField
            fieldName="repetition"
            fieldType="integer"
            fieldValue={String(attempt.repetition)}
          />
          {#if startedAt}<DetailField fieldName="started" fieldType="timestamp"
              >{#snippet value()}<time
                  class="text-sm text-rp-subtle tabular-nums"
                  datetime={startedAt}
                  title={startedAt}>{localTimestamp(startedAt)}</time
                >{/snippet}</DetailField
            >{/if}
          {#if attempt.error}<DetailField
              fieldName="execution error"
              fieldType="string"
              fieldValue={attempt.error}
              multiline
            />{/if}
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
  {:else if tab === 'Assessments'}
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
  {:else}
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
  {/if}
</div>
