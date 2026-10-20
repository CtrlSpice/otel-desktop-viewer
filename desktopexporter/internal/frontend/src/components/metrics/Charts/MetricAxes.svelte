<script lang="ts">
  let {
    padding,
  }: {
    padding: { top: number; right: number; bottom: number; left: number }
  } = $props()
</script>

<span
  class="metric-axes"
  aria-hidden="true"
  style:top="{padding.top - 0.5}px"
  style:right="{padding.right - 0.5}px"
  style:bottom="{padding.bottom - 0.5}px"
  style:left="{padding.left - 0.5}px"
></span>

<style lang="postcss">
  @reference "../../../app.css";

  .metric-axes {
    @apply pointer-events-none absolute z-[1] border-b border-l;
    border-color: var(--color-muted);
    border-bottom-left-radius: var(--radius-xl);
  }
</style>
