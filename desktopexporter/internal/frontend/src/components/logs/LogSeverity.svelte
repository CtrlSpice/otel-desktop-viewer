<script lang="ts">
  import { severityColor, severityLabel } from './log-severity'

  let { number, text }: { number: number; text: string } = $props()
</script>

<span class="log-severity">
  <span
    class="number-circle shrink-0"
    style:--number-circle-color={severityColor(number)}>{number}</span
  >
  <span class="log-severity__label">{severityLabel(text, number)}</span>
</span>

<style lang="postcss">
  @reference "../../app.css";
  .log-severity {
    @apply flex min-w-0 items-center justify-start gap-2 text-left;
  }
  .log-severity__label {
    @apply min-w-0 truncate text-base-content;
  }
</style>
