<script lang="ts">
  import type { Assessment } from './model'
  let {
    assessment,
    maximum = 1,
  }: { assessment?: Assessment; maximum?: number } = $props()
</script>

<span
  class="badge badge-xs badge-soft tabular-nums"
  class:badge-success={assessment?.value === maximum}
  class:badge-warning={assessment?.value != null &&
    assessment.value > 0 &&
    assessment.value < maximum}
  class:badge-error={assessment?.value === 0}
  title={assessment?.value != null
    ? `${assessment.value} / ${maximum}`
    : undefined}>{assessment?.label ?? 'Unscored'}</span
>
