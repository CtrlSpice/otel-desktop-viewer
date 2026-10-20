<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import { cost, traceHref, type EvalRun } from './model'
  import { completionLabel, completionTone } from './group-attempts'
  import type { EvalOutlineRow } from './eval-outline'
  import { formatDuration } from '@/utils/time'

  let {
    run,
    row,
    traceLevel = false,
    parentName,
  }: {
    run: EvalRun
    row: EvalOutlineRow
    traceLevel?: boolean
    parentName?: string
  } = $props()
  const attempt = $derived(row.attempt)
  const members = $derived(
    run.attempts.filter(item => item.caseID === row.caseID)
  )
  const tool = $derived(
    attempt?.tools.find((_, index) => row.id === `${attempt.id}/tool/${index}`)
  )
  const assessments = $derived(
    attempt?.assessments.filter(
      item => row.kind === 'grading' || item.id === row.assessmentID
    ) ?? []
  )
  const prose = $derived(
    row.kind === 'case'
      ? members[0]?.question
      : row.kind === 'execution' && !traceLevel
        ? attempt?.answer
        : tool?.message
  )
  const fields = $derived([
    ...(!traceLevel
      ? [{ name: 'case', value: members[0]?.name ?? row.caseID }]
      : []),
    ...(row.kind === 'case'
      ? [{ name: 'attempts', value: String(members.length) }]
      : []),
    ...(attempt && !traceLevel
      ? [{ name: 'attempt', value: String(attempt.repetition) }]
      : []),
    {
      name: 'total estimate (CAD)',
      value: `${cost(row.estimate.total)}${row.estimate.incomplete && row.estimate.total !== null ? ' · partial' : ''}`,
    },
    ...(traceLevel &&
    (row.kind === 'attempt' || row.kind === 'execution') &&
    attempt?.error
      ? [{ name: 'internal fault', value: attempt.error }]
      : []),
    ...(row.kind === 'execution' && attempt
      ? [
          {
            name: 'tool calls',
            value: `${attempt.tools.length} total · ${attempt.tools.filter(item => item.state === 'Error').length} failed`,
          },
        ]
      : []),
    ...(row.window || row.seconds !== undefined
      ? [
          {
            name: 'duration',
            value: row.window
              ? formatDuration(row.window.endTime - row.window.startTime)
              : `${row.seconds} s`,
          },
        ]
      : []),
  ])
</script>

{#snippet status()}<EvalStatus
    label={completionLabel(row.state, row.internalFault)}
    tone={completionTone(row.state, row.internalFault)}
  />{/snippet}
{#snippet copy()}{#if prose}<ValueCopyButton
      text={prose}
      label={row.name}
    />{/if}{/snippet}
{#snippet supportingFields()}
  <table class="detail-fields w-full" aria-label={`${row.name} fields`}>
    <tbody>
      {#each fields as field}<DetailField
          fieldName={field.name}
          fieldType="string"
          showType={false}
          fieldValue={field.value}
        />{/each}
    </tbody>
  </table>
{/snippet}
{#snippet content()}
  {#if prose}
    {#if traceLevel && tool}
      <table class="detail-fields w-full" aria-label={`${row.name} output`}>
        <tbody>
          <DetailField
            fieldName={tool.state === 'Error' ? 'error' : 'output'}
            fieldType="string"
            showType={false}
            fieldValue={prose}
            multiline
          />
        </tbody>
      </table>
    {:else}<p class="eval-row-detail__prose">{prose}</p>{/if}
  {/if}
  {#if !traceLevel}{@render supportingFields()}{/if}
  {#each assessments as assessment}
    {@const definition = run.assessments.find(
      item => item.id === assessment.id
    )}
    <div class="eval-row-detail__prose">
      <AssessmentBadge {assessment} maximum={definition?.maximum} />
    </div>
    <table
      class="detail-fields w-full"
      aria-label={`${definition?.name ?? assessment.id} explanation`}
    >
      <tbody>
        <DetailField
          fieldName="score"
          fieldType="string"
          showType={false}
          fieldValue={`${assessment.value ?? 'Unscored'} / ${definition?.maximum ?? 1}`}
        />
        <DetailField
          fieldName="explanation"
          fieldType="string"
          showType={false}
          fieldValue={assessment.explanation}
          multiline
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
          multiline
        />
        <DetailField
          fieldName="estimate (CAD)"
          fieldType="string"
          showType={false}
          fieldValue={cost(assessment.cost)}
        />
      </tbody>
    </table>
  {/each}
  {#if traceLevel}{@render supportingFields()}{/if}
{/snippet}
{#snippet telemetry()}
  {#if row.reference}
    <table class="detail-fields w-full" aria-label={`${row.name} telemetry`}>
      <tbody>
        <DetailField
          fieldName="traceId"
          fieldType="bytes"
          fieldValue={row.reference.traceID}
          identifier
        />
        {#if row.reference.spanID}<DetailField
            fieldName="spanId"
            fieldType="bytes"
            fieldValue={row.reference.spanID}
            identifier
          />{/if}
      </tbody>
    </table>
    <p class="eval-row-detail__prose">
      <a class="link link-primary" href={traceHref(row.reference)}
        >Open in Traces</a
      >
    </p>
  {/if}
{/snippet}

{#if traceLevel}
  <section
    class="flex min-h-0 flex-1 flex-col"
    aria-label={`${row.name} details`}
  >
    <PaneHeader
      mode="title"
      title={row.kind === 'case' ? 'Question' : row.name}
      badge={status}
      right={copy}
    />
    <div class="min-h-0 flex-1 overflow-y-auto">
      {#if parentName}<p
          class="eval-row-detail__context truncate"
          title={parentName}
        >
          {parentName}
        </p>{/if}
      {@render content()}
      {@render telemetry()}
    </div>
  </section>
{:else}
  <DetailStack
    label={`${row.name} details`}
    storageKey={`eval-${row.kind}-detail-layout`}
  >
    <DetailBubble label={row.name} trailing={status} action={copy}>
      {@render content()}
    </DetailBubble>
    {#if row.reference}
      <DetailBubble label="Telemetry" initialOpen={false}
        >{@render telemetry()}</DetailBubble
      >
    {/if}
  </DetailStack>
{/if}

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
  }
</style>
