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
  label={state === 'Complete' ? 'Completed' : state}
  tone={state === 'Complete'
    ? 'success'
    : internalError
      ? 'papaya'
      : state === 'Paused'
        ? 'warning'
        : undefined}
  title={internalError ? 'Paused after an internal execution error' : undefined}
/>
