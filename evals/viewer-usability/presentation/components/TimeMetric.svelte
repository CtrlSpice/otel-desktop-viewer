<script lang="ts">
  import { seconds, type Timing } from './model'

  let { value, max }: { value: Timing; max: number } = $props()
  const values = $derived([
    { label: 'Median', status: 'median', seconds: value.medianSeconds },
    { label: 'P95', status: 'p95', seconds: value.p95Seconds },
    { label: 'Slowest', status: 'tail', seconds: value.maxSeconds },
  ])
  const bands = $derived(
    value.medianSeconds === null ||
      value.p95Seconds === null ||
      value.maxSeconds === null
      ? []
      : [
          { status: 'median', seconds: value.medianSeconds },
          { status: 'p95', seconds: value.p95Seconds - value.medianSeconds },
          { status: 'tail', seconds: value.maxSeconds - value.p95Seconds },
        ]
  )
</script>

<div class="metric time">
  <div class="time-distribution">
    <div
      class="time-track"
      role="img"
      aria-label="Time per run: {values
        .map(part => `${part.label}: ${seconds(part.seconds)}`)
        .join('; ')}. Shared scale: 0 to {seconds(max)}."
    >
      {#each bands as part}
        <span
          class="time-{part.status}"
          style="width:{(part.seconds / max) * 100}%"
        ></span>
      {/each}
    </div>
    <div class="time-values">
      {#each values as part}
        <span class="badge badge-soft time-value time-{part.status}"
          ><span>{part.label}</span><strong>{seconds(part.seconds)}</strong
          ></span
        >
      {/each}
    </div>
  </div>
  <div class="time-total">
    <strong aria-label="Total time: {seconds(value.totalSeconds)}"
      >{seconds(value.totalSeconds)}</strong
    >
  </div>
</div>
