<script lang="ts">
  import type { EvalRun } from './model'
  let { state }: { state: EvalRun['state'] } = $props()
</script>

<span
  class="badge badge-xs badge-soft"
  class:badge-success={state === 'Complete'}
  class:badge-warning={state === 'Paused'}
  >{state === 'Complete' ? 'Completed' : state}</span
>
