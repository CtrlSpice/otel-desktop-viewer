<script lang="ts">
  import type { Snippet } from 'svelte'
  import { severityColor, severityLabel } from './log-severity'

  let {
    number,
    text,
    children,
  }: { number: number; text: string; children?: Snippet } = $props()
  let label = $derived(severityLabel(text, number))
</script>

<span class="log-severity" title={label}>
  <span
    class="log-severity__dot size-2 shrink-0 rounded-full"
    style:background-color={severityColor(number)}
    aria-hidden="true"
  ></span>
  <span class="log-severity__label"
    >{#if children}{@render children()}{:else}{label} ({number}){/if}</span
  >
</span>

<style lang="postcss">
  @reference "../../app.css";
  .log-severity {
    @apply inline-flex min-w-0 items-center justify-start gap-2 text-left text-sm tabular-nums;
  }
  .log-severity__label {
    @apply min-w-0 truncate text-base-content;
  }
</style>
