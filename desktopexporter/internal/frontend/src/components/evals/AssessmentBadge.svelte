<script lang="ts">
  import type { Assessment } from './model'
  import EvalStatus from './EvalStatus.svelte'
  let { assessment, maximum }: { assessment?: Assessment; maximum?: number } =
    $props()
</script>

<EvalStatus
  label={assessment?.label ?? 'Unscored'}
  tone={assessment?.pass === true
    ? 'success'
    : assessment?.pass === false
      ? 'error'
      : maximum !== undefined && assessment?.value != null
        ? assessment.value === maximum
          ? 'success'
          : assessment.value === 0
            ? 'error'
            : 'warning'
        : 'neutral'}
  title={assessment?.value != null
    ? `${assessment.value}${maximum === undefined ? '' : ` / ${maximum}`}`
    : undefined}
/>
