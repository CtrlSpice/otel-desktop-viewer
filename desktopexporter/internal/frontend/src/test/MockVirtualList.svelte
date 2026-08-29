<script lang="ts" generics="T">
  import type { Snippet } from 'svelte'
  import { scrollMock } from '@/test/mock-virtual-list'

  type Props = {
    items?: T[]
    renderItem?: Snippet<[item: T, index: number]>
    itemKey?: (item: T, index: number) => string | number
    containerClass?: string
    viewportClass?: string
    viewportLabel?: string
    itemsClass?: string
    defaultEstimatedItemHeight?: number
    bufferSize?: number
  }

  let {
    items = [],
    renderItem,
    itemKey,
    containerClass = 'waterfall-vlist',
    viewportClass = 'waterfall-vlist-viewport',
    viewportLabel = 'Scrollable list',
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

<div class={containerClass}>
  <!-- Matches the real package's keyboard-scrollable viewport. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div
    class={viewportClass}
    role="region"
    tabindex="0"
    aria-label={viewportLabel}
  >
    <div class={itemsClass}>
      {#each items as item, index (itemKey ? itemKey(item, index) : index)}
        <div data-original-index={index}>
          {#if renderItem}
            {@render renderItem(item, index)}
          {/if}
        </div>
      {/each}
    </div>
  </div>
</div>
