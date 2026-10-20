<script lang="ts">
  import type { EvalRun } from './model'
  import EvalStatus from './EvalStatus.svelte'
  let {
    state,
    pauseReason,
  }: {
    state: EvalRun['state']
    pauseReason?: EvalRun['pauseReason']
  } = $props()
  const internalError = $derived(
    state === 'Paused' && pauseReason === 'internal-error'
  )
</script>

<EvalStatus
  label={state}
  tone={state === 'Complete'
    ? 'complete'
    : state === 'Paused'
      ? 'papaya'
      : 'neutral'}
  title={internalError ? 'Paused after an internal execution fault' : undefined}
/>
