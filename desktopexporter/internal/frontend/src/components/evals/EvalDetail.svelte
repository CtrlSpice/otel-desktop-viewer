<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import type { EvalRun } from './model'
  import { cost, estimatedCost, localTimestamp } from './model'
  import RunStatusBadge from './RunStatusBadge.svelte'
  import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
  import { evalTelemetrySummaries } from './eval-telemetry-summaries'
  import type { SpanNode } from '@/types/api-types'
  import type { EvalSourceRunCopy } from './eval-source-copy'
  import type { EvalOutlineRow } from './eval-outline'
  import { referenceKey } from './eval-traces'
  import { formatDuration } from '@/utils/time'
  import { runSummary } from './run-summary'
  import LegendRows from '@/components/shared/LegendRows.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import type { EvalStoredRunSummary } from '@/services/eval-run-types'

  let {
    run,
    row,
    telemetryNodes,
    telemetryLoading = false,
    runCopy,
    storedSummary,
  }: {
    run: EvalRun
    row?: EvalOutlineRow
    telemetryNodes?: Map<string, SpanNode>
    telemetryLoading?: boolean
    runCopy?: EvalSourceRunCopy
    storedSummary?: EvalStoredRunSummary
  } = $props()
  const investigator = $derived(estimatedCost(run.reps, 'investigatorCost'))
  const references = $derived(evalTelemetrySummaries(run))
  const grader = $derived(estimatedCost(run.reps, 'graderCost'))
  const summary = $derived(runSummary(run))
  const outcomes = $derived(storedSummary?.outcomes ?? summary.parts)
  const repCount = $derived(storedSummary?.repCount ?? run.reps.length)
  const cases = $derived(new Set(run.reps.map(rep => rep.caseID)).size)
  const span = $derived(
    run.telemetry
      ? telemetryNodes?.get(referenceKey(run.telemetry))?.spanData
      : undefined
  )
  const duration = $derived(
    row?.window
      ? formatDuration(row.window.endTime - row.window.startTime)
      : run.seconds !== undefined
        ? `${run.seconds} s`
        : undefined
  )
  const showDuration = $derived(
    row?.window
      ? row.window.endTime - row.window.startTime > 1n
      : run.seconds === undefined || run.seconds > 1e-9
  )
  const durationSource = $derived(
    row?.window
      ? span && span.endTime >= span.startTime
        ? 'Recorded span'
        : 'Derived from spans'
      : run.seconds !== undefined
        ? 'Recorded duration'
        : undefined
  )
</script>

<DetailStack label="Experiment details" storageKey="eval-run-detail-layout">
  <DetailBubble label={run.name} layoutKey="Experiment">
    {#snippet labelSuffix()}<span class="text-secondary">· root</span>{/snippet}
    {#snippet trailing()}<RunStatusBadge
        state={run.state}
        pauseReason={run.pauseReason}
      />{/snippet}
    <table class="detail-fields w-full" aria-label="Experiment fields">
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
        {#if run.state !== 'Unknown'}<DetailField
            fieldName="state"
            fieldType="string"
            fieldValue={run.state}
          >
            {#snippet value()}<RunStatusBadge
                state={run.state}
                pauseReason={run.pauseReason}
              />{/snippet}
          </DetailField>{/if}
        {#if run.promptLabel !== undefined}
          <DetailField
            fieldName="prompt"
            fieldType="string"
            fieldValue={run.promptLabel}
            loadText={runCopy?.(run.id, 'prompt')}
            multiline
          />
        {/if}
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
        {#if showDuration}
          <DetailField
            fieldName="duration"
            fieldType="string"
            fieldValue={duration ?? 'Unknown'}
            qualifier={durationSource}
          />
        {/if}
        <DetailField
          fieldName="test cases"
          fieldType="number"
          fieldValue={String(cases)}
        />
        <DetailField
          fieldName="results"
          fieldType="number"
          fieldValue={String(repCount)}
        />
        {#if outcomes.length}
          <DetailField
            fieldName="outcomes"
            fieldType="string"
            fieldValue={outcomes
              .map(part => `${part.label}: ${part.count}`)
              .join(', ')}
          >
            {#snippet value()}
              <LegendRows
                items={outcomes}
                label="Experiment outcomes"
                itemKey={entry => `${entry.tone}:${entry.label}`}
              >
                {#snippet children(entry)}
                  <EvalStatus label={entry.label} tone={entry.tone} />
                  <span class="text-rp-subtle tabular-nums"
                    >[{entry.count}]</span
                  >
                {/snippet}
              </LegendRows>
            {/snippet}
          </DetailField>
        {/if}
      </tbody>
    </table>
  </DetailBubble>
  <DetailBubble
    label="Configuration"
    initialOpen={false}
    count={run.configuration.length}
    countLabel="fields"
  >
    <table class="detail-fields w-full" aria-label="Experiment configuration">
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
    <table
      class="detail-fields w-full"
      aria-label="Grading for this experiment"
    >
      <tbody>
        <DetailField
          fieldName="assessed results"
          fieldType="string"
          fieldValue={`${run.reps.filter(item => item.assessments.length > 0).length} / ${run.reps.length}`}
        />
      </tbody>
    </table>
  </DetailBubble>
  <DetailBubble
    label="Cost"
    initialOpen={false}
    disabled={investigator.total === null && grader.total === null}
  >
    <table class="detail-fields w-full" aria-label="Cost for this experiment">
      <tbody>
        <DetailField
          fieldName="investigator estimate (CAD)"
          fieldType="string"
          fieldValue={`${cost(investigator.total)}${investigator.incomplete && investigator.total !== null ? ' · incomplete' : ''}`}
        />
        <DetailField
          fieldName="grader estimate (CAD)"
          fieldType="string"
          fieldValue={`${cost(grader.total)}${grader.incomplete && grader.total !== null ? ' · incomplete' : ''}`}
        />
      </tbody>
    </table>
  </DetailBubble>
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
