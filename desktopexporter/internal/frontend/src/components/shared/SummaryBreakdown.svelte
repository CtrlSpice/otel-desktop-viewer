<script lang="ts">
  import type { SummaryBreakdownPart } from '@/types/api-types'
  import {
    categoricalPalette,
    metricSeriesPalette,
  } from '@/utils/chart-palette'
  import { themeSignal } from '@/state/theme.svelte'
  import OutcomeBar from './OutcomeBar.svelte'

  let {
    parts,
    signal,
  }: {
    parts: SummaryBreakdownPart[]
    signal: 'trace' | 'metric'
  } = $props()
  const label = $derived(
    signal === 'trace' ? 'Spans by service' : 'Datapoints by series'
  )
  const total = $derived(parts.reduce((sum, part) => sum + part.count, 0))
  const palette = $derived(
    signal === 'trace'
      ? categoricalPalette(25, 'iris', themeSignal.value)
      : metricSeriesPalette(24, 'foam', themeSignal.value)
  )
  function slot(key: string): number {
    let hash = 2166136261
    for (let i = 0; i < key.length; i++)
      hash = Math.imul(hash ^ key.charCodeAt(i), 16777619)
    return (hash >>> 0) % palette.length
  }
  const colouredParts = $derived(
    parts.map(part => ({
      ...part,
      tone: 'neutral' as const,
      color: part.other ? 'var(--color-neutral)' : palette[slot(part.key)],
    }))
  )
</script>

{#if parts.length > 0}
  <div
    class="flex w-full min-w-0 flex-col gap-1.5 border-t border-base-content/5 pt-2 pb-0.5"
  >
    <div class="flex min-w-0 items-center justify-between gap-2 text-xs">
      <span class="truncate">{label}</span>
      <span class="shrink-0 text-base-content tabular-nums"
        >{total.toLocaleString()}</span
      >
    </div>
    <OutcomeBar parts={colouredParts} {label} />
  </div>
{/if}
