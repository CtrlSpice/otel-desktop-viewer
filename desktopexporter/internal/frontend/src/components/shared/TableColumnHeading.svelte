<script lang="ts">
  import { tick } from 'svelte'

  let {
    label,
    source,
    signal,
  }: { label: string; source: string; signal: 'Log' | 'Span' } = $props()
  let external = $derived(source.toLowerCase() !== signal.toLowerCase())
  let sourceLabel = $derived(source.toLowerCase())
  let heading = $state<HTMLSpanElement>()
  let title = $state<HTMLSpanElement>()
  let sourceElement = $state<HTMLSpanElement>()
  let compact = $state(false)

  $effect(() => {
    void label
    void source
    compact = false
    if (!external || !heading || !title || !sourceElement) return
    const row = heading
    const text = title
    const provenance = sourceElement
    let fullSourceWidth = 0
    let active = true
    function fit() {
      if (!active || row.clientWidth === 0) return
      if (!compact) fullSourceWidth = provenance.getBoundingClientRect().width
      compact =
        text.scrollWidth +
          parseFloat(getComputedStyle(row).columnGap) +
          fullSourceWidth >
        row.clientWidth
    }
    const observer = new ResizeObserver(fit)
    observer.observe(row)
    observer.observe(text)
    observer.observe(provenance)
    void tick().then(fit)
    return () => {
      active = false
      observer.disconnect()
    }
  })
</script>

<span class="table-column-heading" bind:this={heading}>
  <span class="table-column-heading__title" bind:this={title} title={label}
    >{label}</span
  >
  {#if external}<span
      class="metadata-origin shrink-0"
      bind:this={sourceElement}
      role="img"
      aria-label={`Source: ${sourceLabel}`}
      title={sourceLabel}>{compact ? sourceLabel.charAt(0) : sourceLabel}</span
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
