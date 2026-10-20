<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import { cost, type EvalRun } from './model'
  import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
  import { evalTelemetrySummaries } from './eval-telemetry-summaries'
  import type { SpanNode } from '@/types/api-types'
  import { completionLabel, completionTone } from './group-attempts'
  import type { EvalOutlineRow } from './eval-outline'
  import { formatDuration } from '@/utils/time'
  import type { SnapshotFieldCopy } from './snapshot-copy'

  let {
    run,
    row,
    traceLevel = false,
    parentName,
    telemetryNodes,
    telemetryLoading = false,
    sourceCopy,
  }: {
    run: EvalRun
    row: EvalOutlineRow
    traceLevel?: boolean
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
    ...(!traceLevel && row.kind !== 'case'
      ? [
          {
            name: 'case',
            value: members[0]?.name ?? row.caseID,
            loadText: members[0]
              ? sourceCopy?.(members[0].id, 'name')
              : undefined,
          },
        ]
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
    attempt?.error !== undefined
      ? [
          {
            name: 'internal fault',
            value: attempt.error,
            loadText: sourceCopy?.(attempt.id, 'error'),
          },
        ]
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
{#snippet copy()}{#if prose !== undefined}<ValueCopyButton
      text={prose}
      loadText={row.kind === 'case' && members[0]
        ? sourceCopy?.(members[0].id, 'question')
        : row.kind === 'execution' && attempt
          ? sourceCopy?.(attempt.id, 'answer')
          : undefined}
      label={row.name}
    />{/if}{/snippet}
{#snippet supportingFields()}
  <table class="detail-fields w-full" aria-label={`${row.name} fields`}>
    <tbody>
      {#each fields as field}<DetailField
          fieldName={field.name}
          fieldType="string"
          fieldValue={field.value}
          loadText={field.loadText}
        />{/each}
    </tbody>
  </table>
{/snippet}
{#snippet content()}
  {#if prose !== undefined}
    {#if traceLevel && tool}
      <table class="detail-fields w-full" aria-label={`${row.name} output`}>
        <tbody>
          <DetailField
            fieldName={tool.state === 'Error' ? 'error' : 'output'}
            fieldType="string"
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
          fieldValue={`${assessment.value ?? 'Unscored'} / ${definition?.maximum ?? 1}`}
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
        <DetailField
          fieldName="estimate (CAD)"
          fieldType="string"
          fieldValue={cost(assessment.cost)}
        />
      </tbody>
    </table>
  {/each}
  {#if traceLevel}{@render supportingFields()}{/if}
{/snippet}
{#snippet telemetry()}
  <EvalTelemetrySummaries
    entries={references}
    nodes={telemetryNodes}
    loading={telemetryLoading}
  />
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
    <DetailBubble
      label={row.kind === 'case' ? 'Question' : row.name}
      trailing={status}
      action={copy}
    >
      {@render content()}
    </DetailBubble>
    {#if references.length}
      <DetailBubble
        label="Telemetry"
        count={references.length}
        countLabel="references"
        initialOpen={false}>{@render telemetry()}</DetailBubble
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
