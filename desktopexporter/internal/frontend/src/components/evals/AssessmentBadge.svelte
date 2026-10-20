<script lang="ts">
  import type { Assessment } from './model'
  import EvalStatus from './EvalStatus.svelte'
  let {
    assessment,
    maximum = 1,
  }: { assessment?: Assessment; maximum?: number } = $props()
</script>

<EvalStatus
  label={assessment?.label ?? 'Unscored'}
  tone={assessment?.value === maximum
    ? 'success'
    : assessment?.value === 0
      ? 'error'
      : assessment?.value != null &&
          assessment.value > 0 &&
          assessment.value < maximum
        ? 'warning'
        : 'neutral'}
  title={assessment?.value != null
    ? `${assessment.value} / ${maximum}`
    : undefined}
/>
