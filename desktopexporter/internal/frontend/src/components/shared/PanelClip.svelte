<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    minimumSize,
    axis = 'width',
    children,
  }: {
    minimumSize: number
    axis?: 'width' | 'height'
    children: Snippet
  } = $props()
</script>

<div class="panel-clip">
  <div
    class="panel-clip__content"
    style:min-width={axis === 'width' ? `${minimumSize}px` : undefined}
    style:min-height={axis === 'height' ? `${minimumSize}px` : undefined}
  >
    {@render children()}
  </div>
</div>

<style lang="postcss">
  @reference "../../app.css";

  .panel-clip {
    @apply relative h-full w-full min-h-0 min-w-0 flex-1 overflow-hidden;
  }

  .panel-clip__content {
    @apply flex h-full min-h-0 min-w-0 flex-col;
  }
</style>
