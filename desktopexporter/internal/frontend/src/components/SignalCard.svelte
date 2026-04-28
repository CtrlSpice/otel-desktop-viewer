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
  class="signal-row {selected ? 'signal-row--selected' : ''}"
  onclick={() => onclick?.(id)}
  aria-pressed={selected}
>
  {#if badge}
    <div class="signal-row__badge">
      {@render badge()}
    </div>
  {/if}

  <div class="signal-row__info">
    <div class="signal-row__title" {title}>{title}</div>
    {#if subtitle}
      <div class="signal-row__subtitle">{subtitle}</div>
    {/if}
  </div>

  {#if spark}
    <div class="signal-row__spark">
      {@render spark()}
    </div>
  {/if}
</button>

<style lang="postcss">
  @reference "../app.css";

  .signal-row {
    @apply flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors duration-100;
    @apply hover:bg-base-200/50;
    cursor: pointer;
  }

  .signal-row:focus-visible {
    outline: var(--focus-ring-width) solid var(--focus-ring-color);
    outline-offset: var(--focus-ring-offset);
  }

  .signal-row--selected {
    @apply bg-primary/[0.07];
    box-shadow: inset 3px 0 0 0 var(--color-primary);
  }

  .signal-row--selected:hover {
    @apply bg-primary/10;
  }

  .signal-row__badge {
    @apply shrink-0;
  }

  .signal-row__info {
    @apply min-w-0 flex-1;
  }

  .signal-row__title {
    @apply truncate text-sm font-medium text-base-content;
  }

  .signal-row__subtitle {
    @apply truncate text-xs text-base-content/50;
  }

  .signal-row__spark {
    @apply h-8 w-20 shrink-0 overflow-hidden;
  }
</style>
