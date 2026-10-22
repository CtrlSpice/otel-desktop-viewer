<script module lang="ts">
  export type BubbleBreakdownPart = {
    key: string
    label: string
    count: number
    color: string
  }
</script>

<script lang="ts">
  import { onDestroy, type Snippet } from 'svelte'
  import LegendRows from './LegendRows.svelte'
  import BreakdownSegment from './BreakdownSegment.svelte'

  let {
    parts,
    defaultKey,
    label,
    barLabel,
    unit,
    showSingleCount = true,
    countFormat = 'detailed',
    showAll = false,
    tooltipOnly = false,
    staticSingle = false,
    subtleLabels = false,
    emptyLabel,
    right,
    onsegmentclick,
  }: {
    parts: BubbleBreakdownPart[]
    defaultKey?: string
    label: string
    barLabel: string
    unit: string
    showSingleCount?: boolean
    countFormat?: 'detailed' | 'compact' | 'bracketed'
    /** Show every label with a decorative bar instead of interactive previews. */
    showAll?: boolean
    /** Keep the breakdown visible while moving labels into hover/focus context. */
    tooltipOnly?: boolean
    /** Keep a sole category's label and count visible without a tooltip. */
    staticSingle?: boolean
    /** Render visible category names and counts as secondary scan information. */
    subtleLabels?: boolean
    emptyLabel?: string
    right?: Snippet
    onsegmentclick?: () => void
  } = $props()
  const colouredParts = $derived(parts.filter(part => part.count > 0))
  let pinnedKey = $state<string | null>(null)
  let hoveredKey = $state<string | null>(null)
  let focusedKey = $state<string | null>(null)
  let hoverTimer: ReturnType<typeof setTimeout> | undefined
  let hoverTarget: string | null = null

  function cancelHoverPreview() {
    clearTimeout(hoverTimer)
    hoverTarget = null
  }

  function previewPart(key: string) {
    if (hoverTarget === key) return
    cancelHoverPreview()
    hoverTarget = key
    hoverTimer = setTimeout(() => {
      hoveredKey = key
    }, 120)
  }

  onDestroy(cancelHoverPreview)
  const activeKey = $derived(
    [hoveredKey, focusedKey, pinnedKey, defaultKey].find(key =>
      colouredParts.some(part => part.key === key)
    ) ?? colouredParts[0]?.key
  )
  const keptKey = $derived(
    colouredParts.find(part => part.key === pinnedKey)?.key ??
      colouredParts.find(part => part.key === defaultKey)?.key ??
      colouredParts[0]?.key
  )
  const activePart = $derived(
    colouredParts.find(part => part.key === activeKey)
  )
  const total = $derived(
    colouredParts.reduce((sum, part) => sum + part.count, 0)
  )
  const percent = new Intl.NumberFormat(undefined, {
    style: 'percent',
    maximumFractionDigits: 1,
  })
  function countLabel(part: BubbleBreakdownPart): string {
    return `${part.count.toLocaleString()} ${unit}${part.count === 1 ? '' : 's'}`
  }
</script>

<div
  class="flow-root min-w-0"
  class:breakdown--subtle-labels={subtleLabels}
  role="group"
  aria-label={label}
  onpointermove={event => {
    if (showAll || event.pointerType === 'touch') return
    const target = event.target
    if (!(target instanceof Element)) return
    const choice = target.closest<HTMLElement>('[data-breakdown-key]')
    if (choice?.dataset.breakdownKey) previewPart(choice.dataset.breakdownKey)
  }}
  onpointerleave={() => {
    cancelHoverPreview()
    hoveredKey = null
  }}
  onfocusout={event => {
    if (
      !(event.relatedTarget instanceof Node) ||
      !event.currentTarget.contains(event.relatedTarget)
    )
      focusedKey = null
  }}
>
  {#if staticSingle && colouredParts.length === 1}
    <div class="breakdown-bar breakdown-bar--single">
      <span class="breakdown-label">{@render partLabel(colouredParts[0])}</span>
    </div>
  {:else if tooltipOnly && colouredParts.length}
    <div
      class="breakdown-bar"
      class:breakdown-bar--single={colouredParts.length === 1}
      role="group"
      aria-label={barLabel}
    >
      {#each colouredParts as part (part.key)}
        <BreakdownSegment
          {part}
          {unit}
          single={colouredParts.length === 1}
          onclick={onsegmentclick}
        />
      {/each}
    </div>
  {:else if colouredParts.length > 1}
    <div
      class="breakdown-bar"
      role="group"
      aria-label={`${barLabel}: ${colouredParts.map(part => `${part.label}: ${countLabel(part)}`).join('; ')}`}
    >
      {#each colouredParts as part (part.key)}
        {#if showAll}
          <span
            class="breakdown-segment"
            style:flex={part.count}
            style:--part-color={part.color}
            aria-hidden="true"
            ><span class="breakdown-segment__fill"></span></span
          >
        {:else}
          <button
            type="button"
            class="breakdown-segment breakdown-segment--interactive"
            data-breakdown-key={part.key}
            style:flex={part.count}
            style:--part-color={part.color}
            aria-label={`${part.label}: ${countLabel(part)}, ${percent.format(part.count / total)} of total`}
            aria-pressed={keptKey === part.key}
            onfocus={() => {
              cancelHoverPreview()
              hoveredKey = null
              focusedKey = part.key
            }}
            onclick={() => {
              cancelHoverPreview()
              hoveredKey = null
              pinnedKey = part.key
            }}
            ><span class="breakdown-segment__fill" aria-hidden="true"
            ></span></button
          >
        {/if}
      {/each}
    </div>
  {/if}

  {#snippet partLabel(part: (typeof colouredParts)[number])}
    <span
      class="breakdown-dot"
      style:background-color={part.color}
      aria-hidden="true"
    ></span>
    <span class="breakdown-label__text">
      <span class="breakdown-label__name">{part.label}</span>
      {#if showSingleCount || colouredParts.length > 1}
        <span class="breakdown-label__count"
          >{#if staticSingle && colouredParts.length === 1}[{part.count.toLocaleString()}]{:else if countFormat === 'compact'}({part.count.toLocaleString()}){:else if countFormat === 'bracketed'}[{part.count.toLocaleString()}]{:else}({countLabel(
              part
            )}{#if colouredParts.length > 1},
              {percent.format(part.count / total)}{/if}){/if}</span
        >
      {/if}
    </span>
  {/snippet}

  {#if !tooltipOnly && !(staticSingle && colouredParts.length === 1) && (activePart || right || emptyLabel)}
    <div
      class="ball-pit"
      class:ball-pit--list={showAll}
      class:ball-pit--after-bar={colouredParts.length > 1}
    >
      <div class="min-w-0 flex-1">
        {#if showAll && colouredParts.length}
          <LegendRows
            items={colouredParts}
            label={`${label} legend`}
            itemKey={part => part.key}
          >
            {#snippet children(part)}
              <span class="breakdown-label">{@render partLabel(part)}</span>
            {/snippet}
          </LegendRows>
        {:else if activePart}
          <div class="breakdown-label">
            {@render partLabel(activePart)}
          </div>
        {:else if emptyLabel}
          <span class="text-sm text-rp-subtle">{emptyLabel}</span>
        {/if}
      </div>
      {#if right}
        <div class="shrink-0">{@render right()}</div>
      {/if}
    </div>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .breakdown-bar {
    @apply flex w-full gap-1;
    height: var(--signal-card-row-h);
  }

  .breakdown-bar--single {
    @apply items-center gap-2 text-sm;
  }

  .breakdown-segment {
    @apply flex min-w-2 items-center rounded-full border-0 bg-transparent p-0;
  }

  .breakdown-segment--interactive {
    @apply cursor-pointer;
  }

  .breakdown-segment__fill {
    @apply pointer-events-none block h-2 w-full rounded-full;
    background-color: var(--part-color);
  }

  .breakdown-segment:focus-visible {
    outline: 2px solid var(--color-primary);
    outline-offset: -2px;
  }

  .ball-pit {
    @apply flex min-w-0 items-center gap-3;
    min-height: var(--signal-card-row-h);
  }

  .ball-pit--after-bar {
    @apply mt-1;
  }

  .ball-pit--list {
    @apply mt-0 items-start;
  }

  .breakdown-label__name {
    @apply min-w-0 truncate;
    color: var(--breakdown-label-color, inherit);
  }

  .breakdown-label__count {
    @apply shrink-0 tabular-nums;
    color: var(--color-subtle);
  }

  .breakdown-label {
    @apply flex min-w-0 items-center gap-1.5 text-sm font-normal text-base-content;
  }

  .breakdown--subtle-labels .breakdown-label {
    color: var(--color-subtle);
  }

  .breakdown-dot {
    @apply size-2 shrink-0 rounded-full;
  }

  .breakdown-label__text {
    @apply flex min-w-0 items-center gap-1.5 overflow-hidden whitespace-nowrap;
  }
</style>
