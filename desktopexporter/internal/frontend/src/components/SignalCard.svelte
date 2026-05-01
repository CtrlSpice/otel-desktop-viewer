<script lang="ts">
  import type { Snippet } from 'svelte'

  type Props = {
    id: string
    selected?: boolean
    title: string
    subtitle?: string
    timestamp?: string
    duration?: string
    badge?: Snippet
    meta: Snippet
    spark?: Snippet
    onclick?: (id: string) => void
    children?: Snippet
  }

  let {
    id,
    selected = false,
    title,
    subtitle,
    timestamp,
    duration,
    badge,
    meta,
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
    <span class="signal-row__title-cluster" {title}>
      <span class="signal-row__title">{title}</span>
      {#if subtitle}
        <span class="signal-row__subtitle">{subtitle}</span>
      {/if}
    </span>
    {#if badge}
      <span class="signal-row__badge">
        {@render badge()}
      </span>
    {/if}
  </div>

  {#if timestamp || duration}
    <div class="signal-row__time">
      <span class="signal-row__timestamp">{timestamp ?? ''}</span>
      <span class="signal-row__duration">{duration ?? ''}</span>
    </div>
  {/if}

  {#if spark}
    <div class="signal-row__spark">
      {@render spark()}
    </div>
  {/if}

  <div class="signal-row__meta-row">
    {@render meta()}
  </div>
</button>

<style lang="postcss">
  @reference "../app.css";

  .signal-row {
    @apply flex w-full flex-col gap-y-1 px-3 py-2.5 text-left text-sm transition-colors duration-100;
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

  .signal-row__title-cluster {
    @apply flex min-w-0 flex-1 items-baseline gap-1;
  }

  .signal-row__title {
    @apply min-w-0 shrink truncate text-sm font-medium text-base-content;
    flex: 0 1 auto;
  }

  .signal-row__subtitle {
    @apply min-w-0 flex-1 truncate text-sm font-normal text-base-content/50;
  }

  .signal-row__badge {
    @apply ml-auto shrink-0;
  }

  .signal-row__time {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    @apply min-w-0 items-baseline gap-x-2 text-sm tabular-nums text-base-content/50;
  }

  .signal-row__timestamp {
    @apply min-w-0 truncate text-left;
  }

  .signal-row__duration {
    @apply shrink-0 text-right tabular-nums;
  }

  /* Secondary facts: trace id, scope + unit, etc. — parents own separators/content. */
  .signal-row__meta-row {
    @apply flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-xs leading-snug text-base-content/45;
  }

  .signal-row__spark {
    @apply h-8 w-full overflow-hidden rounded;
  }
</style>
