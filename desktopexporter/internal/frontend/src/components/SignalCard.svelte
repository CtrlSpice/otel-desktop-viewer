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
  <div class="signal-row__header">
    <span class="signal-row__title" {title}>{title}</span>
    {#if badge}
      <span class="signal-row__badge">
        {@render badge()}
      </span>
    {/if}
  </div>
  {#if subtitle}
    <div class="signal-row__subtitle">{subtitle}</div>
  {/if}

  {#if spark}
    <div class="signal-row__spark">
      {@render spark()}
    </div>
  {/if}
</button>

<style lang="postcss">
  @reference "../app.css";

  .signal-row {
    @apply flex w-full flex-col px-3 py-1.5 text-left transition-colors duration-100;
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

  .signal-row__header {
    @apply flex min-w-0 items-baseline gap-1;
  }

  .signal-row__title {
    @apply truncate text-sm font-medium text-base-content;
  }

  .signal-row__subtitle {
    @apply truncate text-xs text-base-content/50;
  }

  .signal-row__badge {
    @apply ml-auto shrink-0;
  }

  .signal-row__spark {
    @apply mt-1 h-8 w-full overflow-hidden rounded;
  }
</style>
