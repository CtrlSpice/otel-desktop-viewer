<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import type { EvalRun } from './model'
  import { cost, estimatedCost, localTimestamp } from './model'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
  import { evalTelemetrySummaries } from './eval-telemetry-summaries'
  import type { SpanNode } from '@/types/api-types'
  import type { SnapshotRunCopy } from './snapshot-copy'

  let {
    run,
    telemetryNodes,
    telemetryLoading = false,
    runCopy,
  }: {
    run: EvalRun
    telemetryNodes?: Map<string, SpanNode>
    telemetryLoading?: boolean
    runCopy?: SnapshotRunCopy
  } = $props()
  const investigator = $derived(estimatedCost(run.attempts, 'investigatorCost'))
  const references = $derived(evalTelemetrySummaries(run))
  const grader = $derived(estimatedCost(run.attempts, 'graderCost'))
</script>

<DetailStack label="Run details" storageKey="eval-run-detail-layout">
  <DetailBubble label="Details" layoutKey="Run">
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
        >
          {#snippet value()}<RunStatusBadge
              state={run.state}
              pauseReason={run.pauseReason}
            />{/snippet}
        </DetailField>
        <DetailField
          fieldName="source"
          fieldType="string"
          fieldValue={run.source}
          loadText={runCopy?.(run.id, 'source')}
        />
        {#if run.startedAt}
          {@const startedAt = run.startedAt}
          <DetailField
            fieldName="started"
            fieldType="timestamp"
            fieldValue={startedAt}
            timestampLabel={localTimestamp(startedAt)}
          >
            {#snippet value()}<time
                class="text-sm text-base-content tabular-nums"
                datetime={startedAt}
                title={startedAt}>{localTimestamp(startedAt)}</time
              >{/snippet}
          </DetailField>
        {/if}
      </tbody>
    </table>
    <FieldGroup
      label="Configuration"
      detail
      open={false}
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
    </FieldGroup>
    <FieldGroup label="Grading" detail open={false}>
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
  </DetailBubble>
  <DetailBubble label="Telemetry" initialOpen={references.length > 0}>
    {#if references.length}
      <EvalTelemetrySummaries
        entries={references}
        nodes={telemetryNodes}
        loading={telemetryLoading}
      />
    {:else}<p class="px-3 py-2 text-sm text-rp-subtle">
        No telemetry reference recorded for this run.
      </p>{/if}
  </DetailBubble>
</DetailStack>
