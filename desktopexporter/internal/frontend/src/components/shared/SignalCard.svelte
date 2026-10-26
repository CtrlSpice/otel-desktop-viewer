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
    /** Provenance or type information immediately above the title. */
    beforeTitle?: Snippet
    titleContent?: Snippet
    wrapTitle?: boolean
    subtitle?: string
    subtitleContent?: Snippet
    /** Date at the bottom-left, after summary content. */
    date?: Snippet
    /** Optional trailing fact on the date row, such as a duration. */
    dateEnd?: Snippet
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
    /** Subtle line under the title (e.g. a log body preview). */
    description?: string
    singleLineDescription?: boolean
    descriptionIsID?: boolean
    /** Secondary line under header (custom content; prefer description). */
    lead?: Snippet
    meta?: Snippet
    /** Independent control before the card's first heading, outside its button/link. */
    action?: Snippet
    onclick?: (id: string) => void
    onkeydown?: (event: KeyboardEvent) => void
    /** Navigation cards retain native link and open-in-new-tab behaviour. */
    href?: string
    disabled?: boolean
  }

  let {
    id,
    selected = false,
    expanded,
    controls,
    idLine,
    title,
    beforeTitle,
    titleContent,
    wrapTitle = false,
    subtitle,
    subtitleContent,
    date,
    dateEnd,
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
    singleLineDescription = false,
    descriptionIsID = false,
    lead,
    meta,
    action,
    onclick,
    onkeydown,
    href,
    disabled = false,
  }: Props = $props()
</script>

{#snippet cardContent()}
  <div class="signal-row__identity">
    {@render beforeTitle?.()}
    <div
      class="signal-row__header"
      class:signal-row__header--wrap={wrapTitle}
      class:signal-row__header--action={Boolean(action)}
    >
      {#if action}<span class="size-6 shrink-0" aria-hidden="true"></span>{/if}
      <span class="signal-row__title-cluster" {title}>
        <span class="signal-row__title"
          >{#if titleContent}{@render titleContent()}{:else}{title}{/if}</span
        >
        {#if badge && badgePlacement === 'inline'}
          <span class="flex shrink-0 items-center gap-2">{@render badge()}</span
          >
        {/if}
      </span>
      {#if badge && badgePlacement === 'end'}
        <span class="signal-row__badge">
          {@render badge()}
        </span>
      {/if}
    </div>

    {#if subtitleContent}
      {@render subtitleContent()}
    {:else if subtitle}
      <div class="signal-row__subtitle" title={subtitle}>{subtitle}</div>
    {/if}
  </div>

  <div class="signal-row__body">
    {#if description}
      <div class="signal-row__lead">
        <p
          class="signal-row__description"
          class:signal-row__description--single-line={singleLineDescription}
          title={description}
        >
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
                <span class="text-subtle">{timestampSuffix}</span>
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
    {#if date}
      <div class="signal-row__date">
        {@render date()}
        {#if dateEnd}
          <div class="signal-row__date-end">{@render dateEnd()}</div>
        {/if}
      </div>
    {/if}
  </div>
{/snippet}

{#snippet selectableCard()}
  {#if href}
    <a
      href={disabled ? undefined : href}
      role={disabled ? 'link' : undefined}
      aria-disabled={disabled || undefined}
      tabindex={disabled ? -1 : undefined}
      class="signal-row {selected ? 'signal-row--selected' : ''}"
      data-selected={selected || undefined}
      class:item-state={!disabled}>{@render cardContent()}</a
    >
  {:else}
    <button
      type="button"
      {disabled}
      class="signal-row item-state {selected ? 'signal-row--selected' : ''}"
      data-selected={selected || undefined}
      onclick={() => onclick?.(id)}
      {onkeydown}
      aria-pressed={expanded === undefined ? selected : undefined}
      aria-expanded={expanded}
      aria-controls={controls}>{@render cardContent()}</button
    >
  {/if}
{/snippet}

{#if action}
  <div class="signal-card-with-action">
    {@render selectableCard()}
    <div class="signal-card__action">{@render action()}</div>
  </div>
{:else}
  {@render selectableCard()}
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .signal-card-with-action {
    display: grid;
    min-width: 0;
  }
  .signal-card-with-action > .signal-row {
    grid-area: 1 / 1;
  }
  .signal-card__action {
    @apply flex size-6 items-center justify-center;
    grid-area: 1 / 1;
    justify-self: start;
    align-self: start;
    margin: 0.625rem 0 0 var(--pane-inline-inset);
  }
  .signal-card-with-action:hover > .signal-row:not(:disabled) {
    background-color: var(--color-highlight-low);
  }

  .signal-row[aria-disabled='true'] {
    cursor: default;
  }

  .signal-row__body {
    @apply flex min-w-0 flex-col;
  }

  .signal-row :global(.identifier) {
    color: var(--color-subtle);
  }

  .signal-row {
    /* Titles share the card-title size; details use the working size. */
    --item-idle-bg: transparent;
    @apply flex w-full flex-col gap-0 rounded-xl py-2.5 text-left text-sm;
    padding-inline: var(--pane-inline-inset);
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
    @apply flex min-h-[var(--signal-card-row-h)] min-w-0 w-full flex-wrap items-center justify-between gap-x-4 text-sm tabular-nums;
    color: var(--color-subtle);
  }

  .signal-row__date-end {
    @apply flex min-h-[var(--signal-card-row-h)] min-w-0 max-w-full shrink-0 items-center;
  }

  .signal-row__description {
    @apply min-w-0 line-clamp-2 text-sm;
    color: var(--color-subtle);
  }

  .signal-row__description--single-line {
    @apply block truncate;
  }

  .signal-row__header {
    @apply flex h-[var(--signal-card-row-h)] min-w-0 items-center;
    gap: var(--header-gap);
  }

  .signal-row__identity {
    @apply min-w-0;
  }

  .signal-row__header--wrap {
    height: auto;
    min-height: var(--signal-card-row-h);
    align-items: flex-start;
  }

  .signal-row__header--action {
    align-items: center;
    gap: 0;
  }

  .signal-row__header--wrap .signal-row__title {
    white-space: normal;
    overflow-wrap: anywhere;
    overflow: visible;
  }

  .signal-row__title-cluster {
    @apply flex min-w-0 flex-1 items-center;
    gap: var(--header-gap);
  }

  .signal-row__title {
    @apply min-w-0 shrink truncate text-card-title font-normal text-base-content;
    flex: 0 1 auto;
  }

  .signal-row__subtitle {
    @apply min-w-0 truncate text-sm font-normal;
    min-height: var(--signal-card-row-h);
    align-content: center;
    color: var(--color-subtle);
  }

  .signal-row__badge {
    @apply flex max-w-full shrink-0 items-center justify-end;
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
