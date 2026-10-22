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
    onsegmentclick,
  }: {
    parts: OutcomePart[]
    label: string
    right?: Snippet
    onsegmentclick?: () => void
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
  barLabel={`${label} by result count`}
  unit="result"
  emptyLabel="No results"
  tooltipOnly
  staticSingle
  subtleLabels
  countFormat="bracketed"
  {right}
  {onsegmentclick}
/>
