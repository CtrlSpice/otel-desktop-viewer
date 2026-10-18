<script lang="ts">
  import type { Snippet } from 'svelte'

  type Props = {
    id: string
    selected?: boolean
    /** Disclosure cards use expanded state instead of selection semantics. */
    expanded?: boolean
    controls?: string
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
    /** Optional muted suffix after the timestamp in the labeled layout. */
    timestampSuffix?: string
    duration?: string
    /** Duration unit when timeLayout is `interval` (e.g. ms, s). */
    durationUnit?: string
    /** Badges beside the title, right-aligned. */
    badge?: Snippet
    /** Place badges directly after the title or at the row's trailing edge. */
    badgePlacement?: 'inline' | 'end'
    /** Subtle line under the title (e.g. metric description). */
    description?: string
    /** Secondary line under header (custom content; prefer description). */
    lead?: Snippet
    meta?: Snippet
    onclick?: (id: string) => void
  }

  let {
    id,
    selected = false,
    expanded,
    controls,
    idLine,
    title,
    titleMuted = false,
    subtitle,
    timeLayout = 'plain',
    timestampLabel,
    timestamp,
    timestampUnit,
    timestampSuffix,
    duration,
    durationUnit,
    badge,
    badgePlacement = 'end',
    description,
    lead,
    meta,
    onclick,
  }: Props = $props()
</script>

<button
  type="button"
  class="signal-row item-state {selected ? 'signal-row--selected' : ''}"
  onclick={() => onclick?.(id)}
  aria-pressed={expanded === undefined ? selected : undefined}
  aria-expanded={expanded}
  aria-controls={controls}
>
  <div class="signal-row__header">
    <span class="signal-row__title-cluster" {title}>
      <span
        class="signal-row__title"
        class:signal-row__title--muted={titleMuted}>{title}</span
      >
      {#if subtitle}
        <span class="signal-row__subtitle"> ({subtitle})</span>
      {/if}
      {#if badge && badgePlacement === 'inline'}
        <span class="flex shrink-0 items-center gap-1">{@render badge()}</span>
      {/if}
    </span>
    {#if badge && badgePlacement === 'end'}
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
            {#if timestampSuffix}
              <span class="text-rp-muted">{timestampSuffix}</span>
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
    /* Share the card surface with the badge background and title fade. */
    --item-idle-bg: var(--color-base-100);
    --row-surface: var(--item-state-bg);
    @apply flex w-full flex-col gap-0 rounded-xl px-3 py-2.5 text-left text-sm leading-snug;
    cursor: pointer;
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
    @apply relative flex h-[var(--signal-card-row-h)] min-w-0 items-center gap-1;
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

  /* The badges are pinned; a long title slides BENEATH them and is
     occluded -- the same collapse language as the drawer header's tab
     strip under its chrome. Out of flow so the title owns the full row
     width; opaque against the row's composited surface; a short fade on
     the leading edge says the title continues underneath. */
  .signal-row__badge {
    @apply absolute inset-y-0 right-0 flex max-w-[55%] items-center justify-end gap-1;
    background: var(--row-surface);
  }

  .signal-row__badge::before {
    content: '';
    position: absolute;
    right: 100%;
    top: 0;
    bottom: 0;
    width: 1.25rem;
    pointer-events: none;
    background: linear-gradient(to right, transparent, var(--row-surface));
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
