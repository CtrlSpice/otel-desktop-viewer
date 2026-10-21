<script lang="ts" generics="T">
  import { onDestroy, type Snippet } from 'svelte'
  import { legendRowEnds } from './utils/legend-rows'

  let {
    items,
    label,
    itemKey,
    children,
  }: {
    items: T[]
    label: string
    itemKey: (item: T) => string
    children: Snippet<[T]>
  } = $props()
  let availableWidth = $state(0)
  let entryWidths = $state<Record<string, number>>({})
  const pendingWidths = new Map<string, number>()
  let entryFrame: number | undefined
  function measureEntry(
    key: string,
    bounds: DOMRectReadOnly | null | undefined
  ) {
    if (!bounds) return
    pendingWidths.set(key, bounds.width)
    if (entryFrame !== undefined) return
    // Changing categories can move entries between rows. Commit every measured
    // width together outside observer delivery, keyed by the entry's identity.
    entryFrame = requestAnimationFrame(() => {
      entryFrame = undefined
      entryWidths = Object.fromEntries(
        items.map(item => {
          const key = itemKey(item)
          return [key, pendingWidths.get(key) ?? entryWidths[key] ?? 0]
        })
      )
      pendingWidths.clear()
    })
  }
  onDestroy(() => {
    if (entryFrame !== undefined) cancelAnimationFrame(entryFrame)
  })
  let gapWidth = $state(0)
  let element = $state<HTMLDivElement>()
  $effect(() => {
    const node = element
    if (!node) return
    let frame: number | undefined
    const measure = () => {
      frame = undefined
      availableWidth = node.clientWidth
      gapWidth = Number.parseFloat(getComputedStyle(node).columnGap)
    }
    measure()
    // Regrouping entries changes this container's height. Apply its width
    // outside ResizeObserver delivery so that height can be delivered next frame.
    const observer = new ResizeObserver(() => {
      if (frame === undefined) frame = requestAnimationFrame(measure)
    })
    observer.observe(node)
    return () => {
      observer.disconnect()
      if (frame !== undefined) cancelAnimationFrame(frame)
    }
  })
  const rowEnds = $derived(
    legendRowEnds(
      items.map(item => entryWidths[itemKey(item)] ?? 0),
      availableWidth,
      gapWidth
    )
  )
</script>

<div class="legend-rows" bind:this={element} role="list" aria-label={label}>
  {#each rowEnds as end, row}
    {@const start = row === 0 ? 0 : rowEnds[row - 1]!}
    <div class="legend-rows__row" role="presentation">
      {#each items.slice(start, end) as item (itemKey(item))}
        <div
          class="legend-rows__entry"
          role="listitem"
          bind:contentRect={null, bounds => measureEntry(itemKey(item), bounds)}
        >
          {@render children(item)}
        </div>
      {/each}
    </div>
  {/each}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .legend-rows {
    @apply flex min-w-0 flex-col items-start gap-x-4 text-sm font-normal;
    row-gap: var(--legend-row-gap, 0px);
  }
  .legend-rows__row {
    @apply flex items-center;
    column-gap: inherit;
  }
  .legend-rows__entry {
    @apply inline-flex shrink-0 items-center gap-2 whitespace-nowrap tabular-nums;
    min-height: var(--legend-entry-height, var(--signal-card-row-h));
  }
</style>
