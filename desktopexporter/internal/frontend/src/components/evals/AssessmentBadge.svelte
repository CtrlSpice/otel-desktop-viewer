<script lang="ts">
  import type { Assessment } from './model'
  let {
    assessment,
    maximum = 1,
  }: { assessment?: Assessment; maximum?: number } = $props()
</script>

<span
  class="eval-score"
  class:eval-score--good={assessment?.value === maximum}
  class:eval-score--partial={assessment?.value != null &&
    assessment.value > 0 &&
    assessment.value < maximum}
  class:eval-score--bad={assessment?.value === 0}
  title={assessment?.value != null
    ? `${assessment.value} / ${maximum}`
    : undefined}>{assessment?.label ?? '—'}</span
>
