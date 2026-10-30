<script lang="ts">
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import DetailField from '@/components/shared/DetailProperty.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import MarkdownText from '@/components/shared/MarkdownText.svelte'
  import { formatRawJSON } from '@/components/shared/utils/raw-json-format'
  import AssessmentBadge from './AssessmentBadge.svelte'
  import type { Assessment, AssessmentDefinition } from './model'
  import type { EvalSourceTextLoader } from './eval-source-copy'
  import { parseGradingInput } from './grading-input'

  let {
    assessment: requestedAssessment,
    definition: requestedDefinition,
    showScore = true,
    showHeading = true,
    loadRubric: requestedRubric,
    loadExplanation: requestedExplanation,
  }: {
    assessment: Assessment
    definition?: AssessmentDefinition
    showScore?: boolean
    showHeading?: boolean
    loadRubric?: EvalSourceTextLoader
    loadExplanation?: EvalSourceTextLoader
  } = $props()
  let displayed = $state.raw<{
    assessment: Assessment
    definition?: AssessmentDefinition
    loadRubric?: EvalSourceTextLoader
    loadExplanation?: EvalSourceTextLoader
    rubric: string
  }>()
  const assessment = $derived(displayed?.assessment ?? requestedAssessment)
  const definition = $derived(
    displayed ? displayed.definition : requestedDefinition
  )
  const loadRubric = $derived(
    displayed ? displayed.loadRubric : requestedRubric
  )
  const loadExplanation = $derived(
    displayed ? displayed.loadExplanation : requestedExplanation
  )
  const rubric = $derived(displayed?.rubric ?? '')
  let loading = $state(false)
  let requestKey = $state.raw<AbortSignal>()
  let loadError = $state('')
  let instructionsOpen = $state(false)
  const packet = $derived(parseGradingInput(rubric))
  const name = $derived(definition?.name ?? assessment.id)

  $effect(() => {
    const selected = {
      assessment: requestedAssessment,
      definition: requestedDefinition,
      loadRubric: requestedRubric,
      loadExplanation: requestedExplanation,
      rubric: requestedAssessment.rubric,
    }
    loadError = ''
    const loader = requestedRubric
    loading = !!loader
    if (!loader) {
      displayed = selected
      return
    }
    const controller = new AbortController()
    requestKey = controller.signal
    void loader(controller.signal)
      .then(value => {
        if (!controller.signal.aborted)
          displayed = { ...selected, rubric: value }
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
          loadText={loadRubric}
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
    class:eval-assessment-detail--copy={!!(assessment.rubric || loadRubric)}
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
    {#if assessment.rubric || loadRubric}
      <div class="eval-assessment-detail__copy">
        <ValueCopyButton
          text={rubric}
          loadText={loadRubric}
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
