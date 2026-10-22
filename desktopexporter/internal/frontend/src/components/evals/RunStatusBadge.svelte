<script lang="ts">
  import type { EvalRun } from './model'
  import EvalStatus from './EvalStatus.svelte'
  let {
    state,
    pauseReason,
    dotOnly = false,
  }: {
    state: EvalRun['state']
    pauseReason?: EvalRun['pauseReason']
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
