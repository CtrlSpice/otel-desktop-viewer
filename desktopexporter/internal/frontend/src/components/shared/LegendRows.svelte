<script lang="ts" generics="T">
  import type { Snippet } from 'svelte'
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
  let entryBounds = $state<DOMRectReadOnly[]>([])
  let gapWidth = $state(0)
  let element = $state<HTMLDivElement>()
  $effect(() => {
    void availableWidth
    if (element)
      gapWidth = Number.parseFloat(getComputedStyle(element).columnGap)
  })
  const rowEnds = $derived(
    legendRowEnds(
      items.map((_, index) => entryBounds[index]?.width ?? 0),
      availableWidth,
      gapWidth
    )
  )
</script>

<div
  class="legend-rows"
  bind:this={element}
  bind:clientWidth={availableWidth}
  role="list"
  aria-label={label}
>
  {#each rowEnds as end, row}
    {@const start = row === 0 ? 0 : rowEnds[row - 1]!}
    <div class="legend-rows__row" role="presentation">
      {#each items.slice(start, end) as item, index (itemKey(item))}
        <div
          class="legend-rows__entry"
          role="listitem"
          bind:contentRect={entryBounds[start + index]}
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
