<script lang="ts">
  import type { Snippet } from 'svelte'
  import { scrollMock } from '@/test/mock-virtual-list'

  type Props = {
    items?: unknown[]
    renderItem?: Snippet<[item: unknown, index: number]>
    containerClass?: string
    viewportClass?: string
    itemsClass?: string
    defaultEstimatedItemHeight?: number
    bufferSize?: number
  }

  let {
    items = [],
    renderItem,
    viewportClass = 'waterfall-vlist-viewport',
    itemsClass = 'waterfall-vlist-items',
  }: Props = $props()

  export async function scroll(options: {
    index: number
    smoothScroll?: boolean
    shouldThrowOnBounds?: boolean
    align?: string
  }): Promise<void> {
    scrollMock(options)
  }
</script>

<div class="waterfall-vlist">
  <div class={viewportClass}>
    <div class={itemsClass}>
      {#each items as item, index (index)}
        <div data-original-index={index}>
          {#if renderItem}
            {@render renderItem(item, index)}
          {/if}
        </div>
      {/each}
    </div>
  </div>
</div>
