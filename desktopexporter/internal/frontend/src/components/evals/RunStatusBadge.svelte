<script lang="ts">
  import type { EvalRun } from './model'
  let {
    state,
    pauseReason,
  }: { state: EvalRun['state']; pauseReason?: EvalRun['pauseReason'] } =
    $props()
  const internalError = $derived(
    state === 'Paused' && pauseReason === 'internal-error'
  )
</script>

<span
  class="badge badge-xs badge-soft"
  class:badge-success={state === 'Complete'}
  class:badge-warning={state === 'Paused' && !internalError}
  class:badge-papaya={internalError}
  title={internalError ? 'Paused after an internal execution error' : undefined}
  >{state === 'Complete' ? 'Completed' : state}</span
>
