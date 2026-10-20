<script lang="ts">
  import type { Snippet } from 'svelte'
  import Identifier from './Identifier.svelte'

  type Props = {
    id: string
    selected?: boolean
    /** Disclosure cards use expanded state instead of selection semantics. */
    expanded?: boolean
    controls?: string
    /** Optional monospace id line at the bottom of the card (e.g. trace id). */
    idLine?: string
    title: string
    titleContent?: Snippet
    subtitle?: string
    subtitleContent?: Snippet
    /** Independent controls below the title, outside the card selection button. */
    interactiveSubtitle?: Snippet
    /** Full-width date between identity and description. */
    date?: Snippet
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
    descriptionIsID?: boolean
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
    titleContent,
    subtitle,
    subtitleContent,
    interactiveSubtitle,
    date,
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
    descriptionIsID = false,
    lead,
    meta,
    onclick,
  }: Props = $props()
</script>

{#snippet card()}
  <button
    type="button"
    class="signal-row item-state {selected ? 'signal-row--selected' : ''}"
    class:signal-row--interactive-subtitle={!!interactiveSubtitle}
    onclick={() => onclick?.(id)}
    aria-pressed={expanded === undefined ? selected : undefined}
    aria-expanded={expanded}
    aria-controls={controls}
  >
    <div class="signal-row__identity">
      <div class="signal-row__header">
        <span class="signal-row__title-cluster" {title}>
          <span class="signal-row__title"
            >{#if titleContent}{@render titleContent()}{:else}{title}{/if}</span
          >
          {#if badge && badgePlacement === 'inline'}
            <span class="flex shrink-0 items-center gap-2"
              >{@render badge()}</span
            >
          {/if}
        </span>
        {#if badge && badgePlacement === 'end'}
          <span class="signal-row__badge">
            {@render badge()}
          </span>
        {/if}
      </div>

      {#if !interactiveSubtitle && subtitleContent}
        {@render subtitleContent()}
      {:else if !interactiveSubtitle && subtitle}
        <div class="signal-row__subtitle" title={subtitle}>{subtitle}</div>
      {/if}
    </div>

    <div class="signal-row__body">
      {#if date}
        <div class="signal-row__date">{@render date()}</div>
      {/if}

      {#if description}
        <div class="signal-row__lead">
          <p class="signal-row__description" title={description}>
            {#if descriptionIsID}<Identifier
                value={description}
              />{:else}{description}{/if}
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
                  <span class="text-rp-subtle">{timestampSuffix}</span>
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
          <Identifier value={idLine} />
        </div>
      {/if}
    </div>
  </button>
{/snippet}

{#if interactiveSubtitle}
  <div class="signal-card-shell">
    {@render card()}
    <div class="signal-card-shell__subtitle">
      {@render interactiveSubtitle()}
    </div>
  </div>
{:else}
  {@render card()}
{/if}

<style lang="postcss">
  @reference "../../app.css";

  /* A shared grid keeps independent disclosure controls inside the visual card
     without nesting their buttons inside the card's selection button. */
  .signal-card-shell {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto auto auto;
    min-width: 0;
  }

  .signal-row__body {
    @apply flex min-w-0 flex-col;
  }

  .signal-row.signal-row--interactive-subtitle {
    display: grid;
    grid-area: 1 / 1 / 4 / 2;
    grid-template-rows: subgrid;
    min-width: 0;
  }

  .signal-row--interactive-subtitle .signal-row__identity {
    grid-row: 1;
  }

  .signal-row--interactive-subtitle .signal-row__body {
    grid-row: 3;
  }

  .signal-card-shell__subtitle {
    grid-area: 2 / 1;
    z-index: 1;
    min-width: 0;
    margin-inline: 0.75rem;
    pointer-events: none;
  }

  .signal-row :global(.identifier),
  .signal-card-shell__subtitle :global(.identifier) {
    color: var(--color-subtle);
  }

  /* Static label space remains part of the card's click target. Only the
     independent controls and hover titles intercept that selection button. */
  .signal-card-shell__subtitle
    :global(
      :is(button, a, input, select, textarea, label, [tabindex], [title])
    ) {
    pointer-events: auto;
  }

  .signal-row {
    /* Titles use the heading size; details use the working size. */
    --item-idle-bg: var(--color-base-100);
    @apply flex w-full flex-col gap-0 rounded-xl px-3 py-2.5 text-left text-sm;
    cursor: pointer;
  }

  /* Single-line rows: fixed height + flex centering matches detail-cell
     align-middle. Meta keeps min-height so wrapped log bodies can grow. */
  .signal-row__id-line {
    @apply flex h-[var(--signal-card-row-h)] min-w-0 items-center truncate font-sans text-sm;
    color: var(--color-subtle);
  }

  .signal-row__lead {
    @apply flex min-h-[var(--signal-card-row-h)] min-w-0 items-center;
  }

  .signal-row__date {
    @apply min-h-[var(--signal-card-row-h)] min-w-0 w-full text-xs tabular-nums;
    align-content: center;
    color: var(--color-subtle);
  }

  .signal-row__description {
    @apply min-w-0 line-clamp-2 text-sm;
    color: var(--color-subtle);
  }

  .signal-row__header {
    @apply flex h-[var(--signal-card-row-h)] min-w-0 items-center;
    gap: var(--header-gap);
  }

  .signal-row__identity {
    @apply min-w-0;
  }

  .signal-row__title-cluster {
    @apply flex min-w-0 flex-1 items-center;
    gap: var(--header-gap);
  }

  .signal-row__title {
    @apply min-w-0 shrink truncate text-lg font-normal text-base-content;
    flex: 0 1 auto;
  }

  .signal-row__subtitle {
    @apply min-w-0 truncate text-sm font-normal;
    min-height: var(--signal-card-row-h);
    align-content: center;
    color: var(--color-subtle);
  }

  .signal-row__badge {
    @apply flex max-w-[55%] shrink-0 items-center justify-end;
    gap: var(--header-gap);
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
    @apply shrink-0 text-xs;
    color: var(--color-subtle);
  }

  .signal-row__time-value {
    @apply truncate text-base-content;
  }

  .signal-row__time-unit {
    @apply shrink-0 text-base-content;
  }

  .signal-row__timestamp {
    @apply min-w-0 truncate text-left;
  }

  .signal-row__duration {
    @apply shrink-0 tabular-nums;
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
    @apply flex min-h-[var(--signal-card-row-h)] min-w-0 flex-wrap items-center gap-x-2 gap-y-0;
    color: var(--color-subtle);
  }
</style>
