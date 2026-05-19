<script lang="ts">
  type Props = {
    height?: number
    message: string
  }

  let { height = 250, message }: Props = $props()
</script>

<div
  class="metric-chart-empty"
  style:height="{height}px"
  role="status"
>
  {message}
</div>

<style lang="postcss">
  @reference "../app.css";

  .metric-chart-empty {
    @apply flex items-center justify-center text-sm text-base-content/40;
  }
</style>
