<script lang="ts">
  import { tick } from 'svelte'

  let {
    label,
    source,
    signal,
  }: { label: string; source: string; signal: 'Log' | 'Span' } = $props()
  let external = $derived(source.toLowerCase() !== signal.toLowerCase())
  let sourceLabel = $derived(source.charAt(0).toUpperCase() + source.slice(1))
  let heading = $state<HTMLSpanElement>()
  let title = $state<HTMLSpanElement>()
  let badge = $state<HTMLSpanElement>()
  let compact = $state(false)

  $effect(() => {
    void label
    void source
    compact = false
    if (!external || !heading || !title || !badge) return
    const row = heading
    const text = title
    const provenance = badge
    let fullBadgeWidth = 0
    let active = true
    function fit() {
      if (!active || row.clientWidth === 0) return
      if (!compact) fullBadgeWidth = provenance.getBoundingClientRect().width
      compact =
        text.scrollWidth +
          parseFloat(getComputedStyle(row).columnGap) +
          fullBadgeWidth >
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
      class="badge badge-xs badge-soft badge-primary shrink-0 font-normal"
      bind:this={badge}
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
