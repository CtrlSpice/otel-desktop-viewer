<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import { cost, traceHref, type EvalRun } from './model'
  import { completionLabel, completionTone } from './group-attempts'
  import type { EvalOutlineRow } from './eval-outline'
  import { formatDuration } from '@/utils/time'

  let { run, row }: { run: EvalRun; row: EvalOutlineRow } = $props()
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
      : row.kind === 'execution'
        ? attempt?.answer
        : tool?.message
  )
  const fields = $derived([
    { name: 'case', value: members[0]?.name ?? row.caseID },
    ...(row.kind === 'case'
      ? [{ name: 'attempts', value: String(members.length) }]
      : []),
    ...(attempt
      ? [{ name: 'attempt', value: String(attempt.repetition) }]
      : []),
    {
      name: 'total estimate (CAD)',
      value: `${cost(row.estimate.total)}${row.estimate.incomplete && row.estimate.total !== null ? ' · partial' : ''}`,
    },
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

<DetailStack
  label={`${row.name} details`}
  storageKey={`eval-${row.kind}-detail-layout`}
>
  <DetailBubble label={row.name}>
    {#snippet trailing()}<EvalStatus
        label={completionLabel(row.state, row.internalFault)}
        tone={completionTone(row.state, row.internalFault)}
      />{/snippet}
    {#snippet action()}{#if prose}<ValueCopyButton
          text={prose}
          label={row.name}
        />{/if}{/snippet}
    {#if prose}<p class="eval-row-detail__prose">{prose}</p>{/if}
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
  </DetailBubble>
  {#if row.reference}
    <DetailBubble label="Telemetry" initialOpen={false}>
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
    </DetailBubble>
  {/if}
</DetailStack>

<style lang="postcss">
  @reference "../../app.css";
  .eval-row-detail__prose {
    @apply py-2 text-sm text-base-content whitespace-pre-wrap;
    padding-inline: var(--pane-inline-inset);
    overflow-wrap: anywhere;
  }
</style>
