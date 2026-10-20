<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { SummaryBreakdownPart } from '@/types/api-types'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'
  import BubbleBreakdown from './BubbleBreakdown.svelte'

  let {
    parts,
    rootService,
    right,
  }: {
    parts: SummaryBreakdownPart[]
    rootService?: string
    right?: Snippet
  } = $props()
  const rootKey = $derived(
    rootService === undefined ? undefined : `service:${rootService}`
  )
  const colouredParts = $derived(
    parts
      .filter(part => part.count > 0)
      .map(part => ({
        ...part,
        color: part.other
          ? 'var(--color-neutral)'
          : traceServiceColor(
              part.key.slice('service:'.length),
              themeSignal.value
            ),
      }))
      .sort((a, b) => Number(b.key === rootKey) - Number(a.key === rootKey))
  )
</script>

<BubbleBreakdown
  --breakdown-label-color="var(--color-subtle)"
  parts={colouredParts}
  defaultKey={rootKey}
  label="Spans by service"
  barLabel="Service breakdown by span count"
  unit="span"
  showSingleCount={false}
  {right}
/>
