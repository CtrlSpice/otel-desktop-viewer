<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import type { EvalExperiment } from './model'
  import { cost, estimatedCost, localTimestamp } from './model'
  import ExperimentStatusBadge from './ExperimentStatusBadge.svelte'
  import EvalTelemetrySection from './EvalTelemetrySection.svelte'
  import { evalTelemetrySummaries } from './eval-telemetry-summaries'
  import type { SpanNode } from '@/types/api-types'
  import type { EvalSourceExperimentCopy } from './eval-source-copy'
  import type { EvalOutlineRow } from './eval-outline'
  import { referenceKey } from './eval-traces'
  import { formatDuration } from '@/utils/time'
  import { experimentSummary } from './experiment-summary'
  import EvalStatus from './EvalStatus.svelte'
  import type { EvalStoredExperimentSummary } from '@/services/eval-experiment-types'

  let {
    experiment,
    row,
    telemetryNodes,
    telemetryLoading = false,
    experimentCopy,
    storedSummary,
  }: {
    experiment: EvalExperiment
    row?: EvalOutlineRow
    telemetryNodes?: Map<string, SpanNode>
    telemetryLoading?: boolean
    experimentCopy?: EvalSourceExperimentCopy
    storedSummary?: EvalStoredExperimentSummary
  } = $props()
  const investigator = $derived(
    estimatedCost(experiment.results, 'investigatorCost')
  )
  const references = $derived(evalTelemetrySummaries(experiment))
  const grader = $derived(estimatedCost(experiment.results, 'graderCost'))
  const summary = $derived(experimentSummary(experiment))
  const outcomes = $derived(storedSummary?.outcomes ?? summary.parts)
  const resultCount = $derived(
    storedSummary?.resultCount ?? experiment.results.length
  )
  const cases = $derived(
    new Set(experiment.results.map(result => result.caseID)).size
  )
  const span = $derived(
    experiment.telemetry
      ? telemetryNodes?.get(referenceKey(experiment.telemetry))?.spanData
      : undefined
  )
  const duration = $derived(
    row?.window
      ? formatDuration(row.window.endTime - row.window.startTime)
      : experiment.seconds !== undefined
        ? `${experiment.seconds} s`
        : undefined
  )
  const showDuration = $derived(
    row?.window
      ? row.window.endTime - row.window.startTime > 1n
      : experiment.seconds === undefined || experiment.seconds > 1e-9
  )
  const durationSource = $derived(
    row?.window
      ? span && span.endTime >= span.startTime
        ? 'Recorded span'
        : 'Derived from spans'
      : undefined
  )
</script>

{#snippet experimentSection()}
  <DetailBubble label={experiment.name} layoutKey="Experiment">
    {#snippet labelSuffix()}<span class="text-secondary">· experiment</span
      >{/snippet}
    {#snippet trailing()}<span
        class="inline-flex flex-wrap items-center justify-end gap-4"
      >
        <ExperimentStatusBadge
          state={experiment.state}
          pauseReason={experiment.pauseReason}
        />
        {#if outcomes.length}
          <span
            class="inline-flex flex-wrap items-center justify-end gap-4"
            role="list"
            aria-label="Experiment outcomes"
          >
            {#each outcomes as entry}
              <span
                class="inline-flex items-center gap-2 whitespace-nowrap"
                role="listitem"
              >
                <EvalStatus label={entry.label} tone={entry.tone} />
                <span class="text-subtle tabular-nums">[{entry.count}]</span>
              </span>
            {/each}
          </span>
        {/if}
      </span>{/snippet}
    <table class="detail-fields w-full" aria-label="Experiment fields">
      <tbody>
        <DetailField
          fieldName="name"
          fieldType="string"
          fieldValue={experiment.name}
          loadText={experimentCopy?.(experiment.id, 'name')}
        />
        <DetailField
          fieldName="evaluation"
          fieldType="string"
          fieldValue={experiment.evaluation}
          loadText={experimentCopy?.(experiment.id, 'evaluation')}
        />
        {#if experiment.state !== 'Unknown'}<DetailField
            fieldName="state"
            fieldType="enum"
            fieldValue={experiment.state}
          >
            {#snippet value()}<ExperimentStatusBadge
                state={experiment.state}
                pauseReason={experiment.pauseReason}
              />{/snippet}
          </DetailField>{/if}
        {#if experiment.promptLabel !== undefined}
          <DetailField
            fieldName="prompt"
            fieldType="string"
            fieldValue={experiment.promptLabel}
            loadText={experimentCopy?.(experiment.id, 'prompt')}
            multiline
          />
        {/if}
        <DetailField
          fieldName="source"
          fieldType="string"
          fieldValue={experiment.source}
          loadText={experimentCopy?.(experiment.id, 'source')}
        />
        {#if experiment.startedAt}
          {@const startedAt = experiment.startedAt}
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
            fieldType={row?.window ? 'int64' : 'number'}
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
          fieldValue={String(resultCount)}
        />
      </tbody>
    </table>
  </DetailBubble>
{/snippet}
{#snippet configurationSection()}
  <DetailBubble
    label="Configuration"
    initialOpen={false}
    count={experiment.configuration.length}
    countLabel="fields"
  >
    <table class="detail-fields w-full" aria-label="Experiment configuration">
      <tbody
        >{#each experiment.configuration as field}<DetailField
            fieldName={field.name}
            fieldType={field.value === null ? 'null' : typeof field.value}
            fieldValue={String(field.value)}
          />{/each}</tbody
      >
    </table>
  </DetailBubble>
{/snippet}
{#snippet gradingSection()}
  <DetailBubble label="Grading" initialOpen={false}>
    <table
      class="detail-fields w-full"
      aria-label="Grading for this experiment"
    >
      <tbody>
        <DetailField
          fieldName="assessed results"
          fieldType="number"
          fieldValue={`${experiment.results.filter(item => item.assessments.length > 0).length} / ${experiment.results.length}`}
        />
      </tbody>
    </table>
  </DetailBubble>
{/snippet}
{#snippet costSection()}
  <DetailBubble
    label="Cost"
    initialOpen={false}
    disabled={investigator.total === null && grader.total === null}
  >
    <table class="detail-fields w-full" aria-label="Cost for this experiment">
      <tbody>
        <DetailField
          fieldName="investigator API estimate (CAD)"
          fieldType="number"
          fieldValue={`${cost(investigator.total)}${investigator.incomplete && investigator.total !== null ? ' · incomplete' : ''}`}
        />
        <DetailField
          fieldName="grader API estimate (CAD)"
          fieldType="number"
          fieldValue={`${cost(grader.total)}${grader.incomplete && grader.total !== null ? ' · incomplete' : ''}`}
        />
      </tbody>
    </table>
  </DetailBubble>
{/snippet}
{#snippet telemetrySection()}
  <EvalTelemetrySection
    entries={references}
    nodes={telemetryNodes}
    loading={telemetryLoading}
    wholeTraces
    evidenceLogs={[]}
  />
{/snippet}
<!-- Retain the saved layout key across the terminology rename. -->
<DetailStack
  label="Experiment details"
  storageKey="eval-run-detail-layout"
  sections={[
    { id: 'experiment', content: experimentSection },
    { id: 'configuration', content: configurationSection },
    { id: 'grading', content: gradingSection },
    { id: 'cost', content: costSection },
    { id: 'telemetry', content: telemetrySection },
  ]}
/>
