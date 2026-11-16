<script lang="ts">
  import type { EvalExperiment } from './model'
  import EvalStatus from './EvalStatus.svelte'
  let {
    state,
    pauseReason,
    dotOnly = false,
  }: {
    state: EvalExperiment['state']
    pauseReason?: EvalExperiment['pauseReason']
    dotOnly?: boolean
  } = $props()
  const internalError = $derived(
    state === 'Paused' && pauseReason === 'internal-error'
  )
</script>

{#if state !== 'Unknown'}
  <EvalStatus
    {dotOnly}
    label={state}
    tone={state === 'Complete'
      ? 'complete'
      : state === 'Paused'
        ? 'papaya'
        : 'neutral'}
    title={internalError
      ? 'Paused after an internal execution fault'
      : undefined}
  />
{/if}
