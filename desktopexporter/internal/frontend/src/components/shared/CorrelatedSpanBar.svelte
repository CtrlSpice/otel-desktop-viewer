<script lang="ts">
  import type { SpanTiming } from '@/types/api-types'
  import { formatDuration } from '@/utils/time'
  import { spanTimingColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'
  import DurationBar from './DurationBar.svelte'

  let {
    span,
    maximum,
    href,
    origin,
  }: { span: SpanTiming; maximum: bigint; href: string; origin?: bigint } =
    $props()
  const duration = $derived(span.endTime - span.startTime)
  const offset = $derived(span.startTime - (origin ?? span.startTime))
  const label = $derived(formatDuration(duration))
</script>

{#if duration >= 0n}
  <a
    class="correlated-span-bar link link-hover"
    {href}
    aria-label={`Open span ${span.name}, ${label}`}
    title={`${span.name} · ${span.serviceName} · ${label} · Start offset ${formatDuration(offset)}`}
    style:--bar-color={spanTimingColor(span, themeSignal.value)}
  >
    <DurationBar {duration} {maximum} {offset} />
  </a>
{:else}<span title="The stored span ends before it starts"
    >Invalid duration</span
  >{/if}

<style lang="postcss">
  @reference "../../app.css";
  .correlated-span-bar {
    @apply block rounded-sm focus-visible:outline-2 focus-visible:outline-primary;
  }
</style>
