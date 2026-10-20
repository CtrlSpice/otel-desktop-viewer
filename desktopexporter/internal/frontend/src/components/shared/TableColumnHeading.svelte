<script lang="ts">
  let {
    label,
    source,
    signal,
  }: { label: string; source: string; signal: 'Log' | 'Span' } = $props()
  let external = $derived(source.toLowerCase() !== signal.toLowerCase())
  let sourceLabel = $derived(source.toLowerCase())
</script>

<span
  class="table-column-heading"
  class:table-column-heading--external={external}
>
  <span class="table-column-heading__title" title={label}
    >{label}</span
  >
  {#if external}
    <span class="shrink-0 text-rp-subtle" aria-hidden="true">·</span>
    <span
      class="metadata-origin table-column-heading__source"
      role="img"
      aria-label={`Source: ${sourceLabel}`}
      title={sourceLabel}
      ><span>{sourceLabel.slice(0, 1)}</span><span
        class="table-column-heading__source-rest"
        >{sourceLabel.slice(1)}</span
      ></span
    >
  {/if}
</span>

<style lang="postcss">
  @reference "../../app.css";
  .table-column-heading {
    @apply grid min-w-0 items-center gap-1.5 font-normal;
    grid-template-columns: minmax(0, max-content);
    color: var(--table-heading-color);
  }
  .table-column-heading--external {
    grid-template-columns:
      minmax(0, max-content) max-content minmax(min-content, 1fr);
  }
  .table-column-heading__title {
    @apply min-w-0 truncate;
  }
  .table-column-heading__source {
    @apply flex whitespace-nowrap;
  }
  .table-column-heading__source-rest {
    @apply w-0 min-w-0 flex-1 overflow-hidden;
  }
</style>
