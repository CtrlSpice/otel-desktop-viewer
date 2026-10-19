<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowUp02Icon } from '@hugeicons/core-free-icons'

  let {
    visible,
    label,
    onclick,
  }: {
    visible: boolean
    label: string
    onclick: () => void
  } = $props()
  let focused = $state(false)
</script>

{#if visible || focused}
  <button
    type="button"
    class="scroll-to-top btn btn-circle btn-primary"
    aria-label={label}
    title="Scroll to top"
    {onclick}
    onfocus={() => (focused = true)}
    onblur={() => (focused = false)}
  >
    <HugeiconsIcon
      icon={ArrowUp02Icon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />
  </button>
{/if}

<style lang="postcss">
  @reference "../../app.css";
  .scroll-to-top {
    @apply absolute bottom-3 right-6 z-30 size-8 p-0 shadow-lg shadow-primary/25;
  }
</style>
