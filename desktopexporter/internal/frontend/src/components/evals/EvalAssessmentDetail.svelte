<script module lang="ts">
  import type { EvalSourceTextLoader } from './eval-source-copy'

  export type EvalGraderInput = {
    text: string
    contextLabel: string
    loadText?: EvalSourceTextLoader
    available: boolean
  }
</script>

<script lang="ts">
  import { untrack } from 'svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import MarkdownText from '@/components/shared/MarkdownText.svelte'
  import { formatRawJSON } from '@/components/shared/utils/raw-json-format'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import type { Assessment, AssessmentDefinition } from './model'
  import { parseGradingInput } from './grading-input'

  let {
    assessment: requestedAssessment,
    definition: requestedDefinition,
    showScore = true,
    showHeading = true,
    loadRubric: requestedRubric,
    loadExplanation: requestedExplanation,
    contextLabel = requestedAssessment.id,
    externalCopy = false,
    onGraderInput,
  }: {
    assessment: Assessment
    definition?: AssessmentDefinition
    showScore?: boolean
    showHeading?: boolean
    loadRubric?: EvalSourceTextLoader
    loadExplanation?: EvalSourceTextLoader
    contextLabel?: string
    externalCopy?: boolean
    onGraderInput?: (input: EvalGraderInput) => void
  } = $props()
  let displayed = $state.raw<{
    loadRubric?: EvalSourceTextLoader
    contextLabel: string
    rubric: string
    complete: boolean
  }>()
  const assessment = $derived(requestedAssessment)
  const definition = $derived(requestedDefinition)
  const loadRubric = $derived(
    displayed ? displayed.loadRubric : requestedRubric
  )
  const loadExplanation = $derived(requestedExplanation)
  const rubric = $derived(displayed?.rubric ?? requestedAssessment.rubric)
  // A completed packet is already exact. Copy must not re-read a newer source.
  const rubricCopy = $derived(displayed?.complete ? undefined : loadRubric)
  $effect(() => {
    onGraderInput?.({
      text: rubric,
      contextLabel: displayed?.contextLabel ?? contextLabel,
      loadText: rubricCopy,
      available: !!(rubric || loadRubric),
    })
  })
  let loading = $state(false)
  let requestKey = $state.raw<AbortSignal>()
  let loadError = $state('')
  let instructionsOpen = $state(false)
  const packet = $derived(parseGradingInput(rubric))
  const name = $derived(definition?.name ?? assessment.id)

  $effect(() => {
    const selected = {
      loadRubric: requestedRubric,
      contextLabel,
      rubric: requestedAssessment.rubric,
      complete: !requestedRubric,
    }
    loadError = ''
    const loader = requestedRubric
    loading = !!loader
    if (!untrack(() => displayed)) displayed = selected
    if (!loader) {
      displayed = selected
      return
    }
    const controller = new AbortController()
    requestKey = controller.signal
    void loader(controller.signal)
      .then(value => {
        if (!controller.signal.aborted)
          displayed = { ...selected, rubric: value, complete: true }
      })
      .catch(() => {
        if (!controller.signal.aborted)
          loadError = 'Could not load the full grader input.'
      })
      .finally(() => {
        if (!controller.signal.aborted) loading = false
      })
    return () => controller.abort()
  })
</script>

{#snippet content()}
  <table class="detail-fields w-full" aria-label={`${name} explanation`}>
    <tbody>
      {#if showScore}
        <DetailField
          fieldName="score"
          fieldType="number"
          fieldValue={assessment.value === null
            ? 'Unscored'
            : `${assessment.value}${definition?.maximum !== undefined ? ` / ${definition.maximum}` : ''}`}
        />
        {#if assessment.explanation}
          <DetailField
            fieldName="explanation"
            fieldType="string"
            fieldValue={assessment.explanation}
            loadText={loadExplanation}
            sourceKey={assessment}
            multiline
          >
            {#snippet value()}<MarkdownText
                text={assessment.explanation}
              />{/snippet}
          </DetailField>
        {/if}
      {/if}
      {#if assessment.evaluator}<DetailField
          fieldName="evaluator"
          fieldType="string"
          fieldValue={assessment.evaluator}
        />{/if}
    </tbody>
  </table>
  <table
    class="detail-fields w-full"
    aria-label="Grader input"
    hidden={!rubric && !loadRubric}
  >
    <caption
      class="px-[var(--pane-inline-inset)] py-2 text-left text-sm font-normal text-subtle"
    >
      <span class="flex items-center justify-between gap-2">
        <span>Grader input · {displayed?.contextLabel ?? contextLabel}</span>
        {#if !showHeading && !externalCopy && (rubric || loadRubric)}
          <ValueCopyButton
            text={rubric}
            loadText={rubricCopy}
            sourceKey={displayed}
            label="grader input"
          />
        {/if}
      </span>
    </caption>
    <tbody>
      {#if packet}
        {#each packet.criteria as criterion}
          <DetailField
            fieldName="criterion"
            qualifier={criterion.id}
            fieldType="string"
            fieldValue={criterion.text}
            multiline
          >
            {#snippet value()}<MarkdownText text={criterion.text} />{/snippet}
          </DetailField>
          {#each Object.entries(criterion.scoring) as [label, description]}
            <DetailField
              fieldName={label}
              qualifier="scoring"
              fieldType="string"
              fieldValue={description}
              multiline
            >
              {#snippet value()}<MarkdownText text={description} />{/snippet}
            </DetailField>
          {/each}
        {/each}
      {:else if rubric}
        <DetailField
          fieldName="criterion"
          fieldType="string"
          fieldValue={rubric}
          loadText={rubricCopy}
          sourceKey={displayed}
          multiline
        >
          {#snippet value()}<MarkdownText text={rubric} />{/snippet}
        </DetailField>
      {/if}
    </tbody>
  </table>
  <LoadingStatus
    pending={loading}
    label="Loading grader input…"
    {requestKey}
    class="block px-3 py-2 text-sm text-subtle"
  />
  {#if loadError}<p class="px-3 py-2 text-sm" role="alert">{loadError}</p>{/if}
  {#if packet}
    {#if packet.reference !== undefined || packet.evidence !== undefined || packet.artifact !== undefined}
      <table class="detail-fields w-full" aria-label="Assessment evidence">
        <tbody>
          {#if packet.reference !== undefined}
            <DetailField
              fieldName="reference"
              fieldType="string"
              fieldValue={packet.reference}
              multiline
            >
              {#snippet value()}<pre
                  class="whitespace-pre-wrap font-mono text-sm">{packet.reference}</pre>{/snippet}
            </DetailField>
          {/if}
          {#each [['evidence', packet.evidence], ['artifact', packet.artifact]] as [label, json]}
            {#if json !== undefined}
              <DetailField
                fieldName={label!}
                fieldType="JSON"
                fieldValue={json}
                jsonText={formatRawJSON(json)}
              />
            {/if}
          {/each}
        </tbody>
      </table>
    {/if}
    {#if packet.instructions || packet.viewerSkill}
      <FieldGroup
        label="Grader instructions"
        detail
        bind:open={instructionsOpen}
      >
        {#if instructionsOpen}
          <table class="detail-fields w-full" aria-label="Grader instructions">
            <tbody>
              {#if packet.instructions}<DetailField
                  fieldName="instructions"
                  fieldType="string"
                  fieldValue={packet.instructions}
                  multiline
                >
                  {#snippet value()}<MarkdownText
                      text={packet.instructions}
                    />{/snippet}
                </DetailField>{/if}
              {#if packet.viewerSkill}<DetailField
                  fieldName="viewer skill"
                  fieldType="string"
                  fieldValue={packet.viewerSkill}
                  multiline
                >
                  {#snippet value()}<MarkdownText
                      text={packet.viewerSkill!}
                    />{/snippet}
                </DetailField>{/if}
            </tbody>
          </table>
        {/if}
      </FieldGroup>
    {/if}
  {/if}
{/snippet}

{#if showHeading}
  <div
    class="eval-assessment-detail"
    class:eval-assessment-detail--copy={!!(rubric || loadRubric)}
  >
    <FieldGroup label={name} detail>
      {#snippet heading()}
        <span>{name}</span>
        {#if showScore}<AssessmentBadge
            {assessment}
            maximum={definition?.maximum}
          />{/if}
      {/snippet}
      {@render content()}
    </FieldGroup>
    {#if !externalCopy && (rubric || loadRubric)}
      <div
        class="eval-assessment-detail__copy"
        title={`Grader input · ${displayed?.contextLabel ?? contextLabel}`}
      >
        <ValueCopyButton
          text={rubric}
          loadText={rubricCopy}
          sourceKey={displayed}
          label="grader input"
        />
      </div>
    {/if}
  </div>
{:else}
  {@render content()}
{/if}

<style lang="postcss">
  .eval-assessment-detail {
    position: relative;
  }
  .eval-assessment-detail--copy
    > :global(.field-group)
    > :global(.field-group__heading) {
    padding-right: calc(var(--pane-inline-inset) + 2.5rem);
  }
  .eval-assessment-detail__copy {
    position: absolute;
    top: 0.25rem;
    right: var(--pane-inline-inset);
  }
</style>
