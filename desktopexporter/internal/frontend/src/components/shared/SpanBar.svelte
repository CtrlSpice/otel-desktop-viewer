<script lang="ts">
  let {
    offsetPercent = 0,
    widthPercent,
    class: className = '',
  }: {
    offsetPercent?: number
    widthPercent: number
    class?: string
  } = $props()
</script>

<span
  class="span-bar {className}"
  style:left={`${offsetPercent}%`}
  style:width={`${widthPercent}%`}
  aria-hidden="true"
></span>

<style lang="postcss">
  @reference "../../app.css";
  .span-bar {
    @apply absolute z-[1] rounded-full top-1/2 -translate-y-1/2 border-0 overflow-hidden;
    height: var(--waterfall-bar-height, 0.875rem);
    min-width: 2px;
    background-color: var(--bar-color);
    transition: background-color 0.12s ease;
  }
</style>
