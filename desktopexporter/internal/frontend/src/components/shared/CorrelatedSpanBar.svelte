<script lang="ts">
  import type { SpanTiming } from '@/types/api-types'
  import { formatDuration } from '@/utils/time'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'
  import SpanBar from './SpanBar.svelte'

  let {
    span,
    maximum,
    href,
  }: { span: SpanTiming; maximum: bigint; href: string } = $props()
  const duration = $derived(span.endTime - span.startTime)
  const label = $derived(formatDuration(duration))
  const width = $derived(
    maximum > 0n
      ? Math.min(100, Number((duration * 10000n) / maximum) / 100)
      : 0
  )
</script>

{#if duration >= 0n}
  <a
    class="correlated-span-bar link link-hover"
    {href}
    aria-label={`Open span ${span.name}, ${label}`}
    title={`${span.name} · ${span.serviceName} · ${label}`}
    style:--bar-color={span.statusCodeValue === 2
      ? 'var(--color-error)'
      : traceServiceColor(span.serviceName, themeSignal.value)}
  >
    <span class="correlated-span-bar__track">
      <SpanBar widthPercent={width} />
      <span
        class="correlated-span-bar__duration"
        style:left={`calc(${width}% + 1rem)`}>{label}</span
      >
    </span>
  </a>
{:else}<span title="The stored span ends before it starts"
    >Invalid duration</span
  >{/if}

<style lang="postcss">
  @reference "../../app.css";
  .correlated-span-bar {
    @apply relative block min-h-[var(--signal-card-row-h)] min-w-0 rounded-sm focus-visible:outline-2 focus-visible:outline-primary;
  }
  .correlated-span-bar__track {
    @apply absolute inset-y-0 left-0 right-24;
  }
  .correlated-span-bar__duration {
    @apply absolute top-1/2 -translate-y-1/2 whitespace-nowrap tabular-nums;
  }
</style>
