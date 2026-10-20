<script lang="ts">
  let {
    label,
    source,
    signal,
  }: { label: string; source: string; signal: 'Log' | 'Span' } = $props()
  let external = $derived(source.toLowerCase() !== signal.toLowerCase())
  let sourceLabel = $derived(source.toLowerCase())
</script>

<span class="table-column-heading">
  <span class="table-column-heading__title" title={label}
    >{label}</span
  >
  {#if external}<span
      class="metadata-origin min-w-0 truncate"
      role="img"
      aria-label={`Source: ${sourceLabel}`}
      title={sourceLabel}>{sourceLabel}</span
    >{/if}
</span>

<style lang="postcss">
  @reference "../../app.css";
  .table-column-heading {
    @apply flex min-w-0 items-center gap-1.5;
    color: var(--table-heading-color);
  }
  .table-column-heading__title {
    @apply min-w-0 truncate;
  }
</style>
