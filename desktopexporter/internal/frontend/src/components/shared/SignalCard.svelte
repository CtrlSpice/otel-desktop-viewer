<script lang="ts">
  import type { Snippet } from 'svelte'

  type Props = {
    id: string
    selected?: boolean
    /** Optional monospace id line at the bottom of the card (e.g. trace id). */
    idLine?: string
    title: string
    /** Italic muted title (e.g. placeholder while data is still arriving). */
    titleMuted?: boolean
    subtitle?: string
    /** `plain`: value grid. `interval`: Start + Duration (traces). `labeled`: one labeled timestamp (logs/metrics). */
    timeLayout?: 'plain' | 'interval' | 'labeled'
    /** Label for `labeled` layout (e.g. "Timestamp:", "Last seen:"). */
    timestampLabel?: string
    timestamp?: string
    /** Timezone suffix when timeLayout is `interval` (e.g. UTC, PST). */
    timestampUnit?: string
    duration?: string
    /** Duration unit when timeLayout is `interval` (e.g. ms, s). */
    durationUnit?: string
    /** Badges beside the title, right-aligned. */
    badge?: Snippet
    /** Subtle line under the title (e.g. metric description). */
    description?: string
    /** Secondary line under header (custom content; prefer description). */
    lead?: Snippet
    meta?: Snippet
    onclick?: (id: string) => void
    children?: Snippet
  }

  let {
    id,
    selected = false,
    idLine,
    title,
    titleMuted = false,
    subtitle,
    timeLayout = 'plain',
    timestampLabel,
    timestamp,
    timestampUnit,
    duration,
    durationUnit,
    badge,
    description,
    lead,
    meta,
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
      <span
        class="signal-row__title"
        class:signal-row__title--muted={titleMuted}
      >{title}</span>
      {#if subtitle}
        <span class="signal-row__subtitle"> ({subtitle})</span>
      {/if}
    </span>
    {#if badge}
      <span class="signal-row__badge">
        {@render badge()}
      </span>
    {/if}
  </div>

  {#if description}
    <div class="signal-row__lead">
      <p class="signal-row__description" title={description}>
        {description}
      </p>
    </div>
  {:else if lead}
    <div class="signal-row__lead">
      {@render lead()}
    </div>
  {/if}

  {#if timestamp || duration}
    {#if timeLayout === 'interval'}
      {#if timestamp}
        <div class="signal-row__time signal-row__time--interval">
          <span class="signal-row__time-group">
            <span class="signal-row__time-label">Start:</span>
            <span class="signal-row__time-value signal-row__timestamp"
              >{timestamp}</span
            >
            {#if timestampUnit}
              <span class="signal-row__time-unit">{timestampUnit}</span>
            {/if}
          </span>
        </div>
      {/if}
      {#if duration}
        <div class="signal-row__time signal-row__time--interval">
          <span class="signal-row__time-group">
            <span class="signal-row__time-label">Duration:</span>
            <span class="signal-row__time-value signal-row__duration"
              >{duration}</span
            >
            {#if durationUnit}
              <span class="signal-row__time-unit">{durationUnit}</span>
            {/if}
          </span>
        </div>
      {/if}
    {:else if timeLayout === 'labeled'}
      <div class="signal-row__time signal-row__time--interval">
        {#if timestamp}
          <span class="signal-row__time-group">
            {#if timestampLabel}
              <span class="signal-row__time-label">{timestampLabel}</span>
            {/if}
            <span class="signal-row__time-value signal-row__timestamp"
              >{timestamp}</span
            >
            {#if timestampUnit}
              <span class="signal-row__time-unit">{timestampUnit}</span>
            {/if}
          </span>
        {/if}
      </div>
    {:else}
      <div class="signal-row__time">
        <span class="signal-row__timestamp">{timestamp ?? ''}</span>
        <span class="signal-row__duration">{duration ?? ''}</span>
      </div>
    {/if}
  {/if}

  {#if meta}
    <div class="signal-row__meta-row">
      {@render meta()}
    </div>
  {/if}

  {#if idLine}
    <div class="signal-row__id-line" title={idLine}>
      {idLine}
    </div>
  {/if}
</button>

<style lang="postcss">
  @reference "../../app.css";

  .signal-row {
    /*
     * Drawer card typography: text-sm default (title, subtitle, description,
     * timestamps, values, meta). Labels → text-xs (.signal-row__label,
     * .signal-row__time-label). Badges → badge-xs on the snippet markup.
     */
    --signal-card-row-h: 1.5rem;
    @apply flex w-full flex-col gap-0 px-3 py-1.5 text-left text-sm leading-snug transition-colors duration-100;
    @apply hover:bg-base-200/50;
    cursor: pointer;
  }

  .signal-row:focus-visible {
    outline: var(--focus-ring-width) solid var(--focus-ring-color);
    outline-offset: var(--focus-ring-offset);
  }

  .signal-row--selected {
    @apply bg-primary/[0.07];
    box-shadow: inset 2px 0 0 0 var(--color-primary);
  }

  .signal-row--selected:hover {
    @apply bg-primary/10;
  }

  /* Single-line rows: fixed height + flex centering matches detail-cell
     align-middle. Meta keeps min-height so wrapped log bodies can grow. */
  .signal-row__id-line {
    @apply flex h-[var(--signal-card-row-h)] min-w-0 items-center truncate font-mono text-sm;
    color: var(--color-subtle);
  }

  .signal-row__lead {
    @apply flex min-h-[var(--signal-card-row-h)] min-w-0 items-center;
  }

  .signal-row__description {
    @apply min-w-0 line-clamp-2 text-sm leading-tight;
    color: var(--color-subtle);
  }

  .signal-row__header {
    @apply flex h-[var(--signal-card-row-h)] min-w-0 items-center gap-1;
  }

  .signal-row__title-cluster {
    @apply flex min-w-0 flex-1 items-center gap-1;
  }

  .signal-row__title {
    @apply min-w-0 shrink truncate text-sm font-medium leading-none text-base-content;
    flex: 0 1 auto;
  }

  .signal-row__title--muted {
    @apply font-normal italic;
    color: var(--color-muted);
  }

  .signal-row__subtitle {
    @apply min-w-0 flex-1 truncate text-sm font-normal leading-none;
    color: var(--color-subtle);
  }

  .signal-row__badge {
    @apply ml-auto flex max-w-[55%] shrink-0 flex-wrap items-center justify-end gap-1;
  }

  .signal-row__time {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    @apply h-[var(--signal-card-row-h)] min-w-0 gap-x-2 tabular-nums;
    color: var(--color-subtle);
  }

  .signal-row__time--interval {
    display: flex;
    @apply min-w-0 gap-x-4 overflow-hidden;
  }

  .signal-row__time-group {
    @apply inline-flex min-w-0 items-center gap-x-1;
  }

  .signal-row__time-group:first-child {
    @apply min-w-0 flex-1;
  }

  .signal-row__time-group:last-child {
    @apply shrink-0;
  }

  /* Shared label style for time rows and meta snippets (e.g. Last value:). */
  :global(.signal-row__label),
  .signal-row__time-label {
    @apply shrink-0 text-xs leading-none;
    color: var(--color-subtle);
  }

  .signal-row__time-value {
    @apply truncate leading-none text-base-content;
  }

  .signal-row__time-unit {
    @apply shrink-0 leading-none text-base-content;
  }

  .signal-row__timestamp {
    @apply min-w-0 truncate text-left leading-none;
  }

  .signal-row__duration {
    @apply shrink-0 tabular-nums leading-none;
  }

  .signal-row__time:not(.signal-row__time--interval) .signal-row__timestamp {
    color: inherit;
  }

  .signal-row__time:not(.signal-row__time--interval) .signal-row__duration {
    @apply text-right;
    color: inherit;
  }

  /* Secondary facts: trace id, scope + unit, etc. — parents own separators/content. */
  .signal-row__meta-row {
    @apply flex min-h-[var(--signal-card-row-h)] min-w-0 flex-wrap items-center gap-x-2 gap-y-0 leading-snug;
    color: var(--color-subtle);
  }

</style>
