<script module lang="ts">
  export type OutcomePart = {
    label: string
    count: number
    tone: 'success' | 'warning' | 'error' | 'papaya' | 'neutral'
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte'
  import BubbleBreakdown from './BubbleBreakdown.svelte'

  let {
    parts,
    label,
    right,
  }: {
    parts: OutcomePart[]
    label: string
    right?: Snippet
  } = $props()
  const colouredParts = $derived(
    parts.map(part => ({
      ...part,
      key: `${part.tone}:${part.label}`,
      color: `var(--color-${part.tone === 'neutral' ? 'base-300' : part.tone === 'success' ? 'accent' : part.tone})`,
    }))
  )
</script>

<BubbleBreakdown
  parts={colouredParts}
  label={`${label} outcomes`}
  barLabel={`${label} by attempt count`}
  unit="attempt"
  emptyLabel="No results"
  showAll
  tooltipOnly
  countFormat="compact"
  {right}
/>
