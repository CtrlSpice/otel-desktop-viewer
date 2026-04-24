<script lang="ts">
  import type { Snippet } from 'svelte'

  type Props = {
    id: string
    selected?: boolean
    title: string
    subtitle?: string
    badge?: Snippet
    spark?: Snippet
    onclick?: (id: string) => void
  }

  let {
    id,
    selected = false,
    title,
    subtitle,
    badge,
    spark,
    onclick,
  }: Props = $props()
</script>

<button
  type="button"
  class="signal-card {selected ? 'signal-card--selected' : ''}"
  onclick={() => onclick?.(id)}
  aria-pressed={selected}
>
  <div class="signal-card__header">
    <span class="signal-card__title" {title}>{title}</span>
    {#if badge}
      <span class="signal-card__badge">
        {@render badge()}
      </span>
    {/if}
  </div>

  {#if subtitle}
    <span class="signal-card__subtitle">{subtitle}</span>
  {/if}

  {#if spark}
    <div class="signal-card__spark">
      {@render spark()}
    </div>
  {/if}
</button>

<style lang="postcss">
  @reference "../app.css";

  .signal-card {
    @apply flex w-full flex-col gap-1 rounded-lg border border-base-300/40 bg-base-100/60 px-3 py-2 text-left transition-[background-color,border-color,box-shadow] duration-150;
    @apply hover:border-base-300/70 hover:bg-base-200/50;
    cursor: pointer;
  }

  .signal-card:focus-visible {
    outline: var(--focus-ring-width) solid var(--focus-ring-color);
    outline-offset: var(--focus-ring-offset);
  }

  .signal-card--selected {
    @apply border-primary/40 bg-primary/[0.07];
    box-shadow: inset 2px 0 0 0 var(--color-primary);
  }

  .signal-card--selected:hover {
    @apply border-primary/50 bg-primary/10;
  }

  .signal-card__header {
    @apply flex min-w-0 items-center gap-2;
  }

  .signal-card__title {
    @apply min-w-0 flex-1 truncate text-sm font-medium text-base-content;
  }

  .signal-card__badge {
    @apply shrink-0;
  }

  .signal-card__subtitle {
    @apply truncate text-xs text-base-content/50;
  }

  .signal-card__spark {
    @apply mt-0.5 h-8 w-full overflow-hidden;
  }
</style>
