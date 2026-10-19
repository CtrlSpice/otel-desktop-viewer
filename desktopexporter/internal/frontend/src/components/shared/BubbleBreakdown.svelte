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

  let {
    parts,
    defaultKey,
    label,
    barLabel,
    unit,
    showSingleCount = true,
    emptyLabel,
    right,
  }: {
    parts: BubbleBreakdownPart[]
    defaultKey?: string
    label: string
    barLabel: string
    unit: string
    showSingleCount?: boolean
    emptyLabel?: string
    right?: Snippet
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
  role="group"
  aria-label={label}
  onpointermove={event => {
    if (event.pointerType === 'touch') return
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
  {#if colouredParts.length > 1}
    <div
      class="mt-2 flex h-2 w-full gap-1"
      role="group"
      aria-label={`${barLabel}: ${colouredParts.map(part => `${part.label}: ${countLabel(part)}`).join('; ')}`}
    >
      {#each colouredParts as part (part.key)}
        <button
          type="button"
          class="breakdown-segment min-w-2 rounded-full"
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
        ></button>
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
      <span class="min-w-0 truncate">{part.label}</span>
      {#if showSingleCount || colouredParts.length > 1}
        <span class="shrink-0 text-rp-subtle tabular-nums"
          >({countLabel(part)}{#if colouredParts.length > 1},
            {percent.format(part.count / total)}{/if})</span
        >
      {/if}
    </span>
  {/snippet}

  {#if activePart || right || emptyLabel}
    <div class="ball-pit">
      <div class="min-w-0 flex-1">
        {#if activePart}
          {#key activeKey}
            <div class="breakdown-label">
              {@render partLabel(activePart)}
            </div>
          {/key}
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

  .breakdown-segment {
    @apply cursor-pointer border-0 p-0;
    background-color: color-mix(
      in srgb,
      var(--part-color) calc(var(--waterfall-bar-opacity) * 100%),
      transparent
    );
  }

  .breakdown-segment:focus-visible {
    outline: 2px solid var(--color-primary);
    outline-offset: -2px;
  }

  :global(html[data-theme='rose-pine-dawn']) .breakdown-segment {
    border: 1px solid var(--part-color);
  }

  .ball-pit {
    @apply mt-1 flex h-7 min-w-0 items-center gap-3;
  }

  .breakdown-label {
    @apply flex min-w-0 items-center gap-1.5 text-sm font-normal text-base-content;
    animation: breakdown-label-in 140ms ease-out;
  }

  .breakdown-dot {
    @apply size-2 shrink-0 rounded-full;
  }

  .breakdown-label__text {
    @apply flex min-w-0 items-center gap-1.5 overflow-hidden whitespace-nowrap;
  }

  @keyframes breakdown-label-in {
    from {
      opacity: 0.5;
    }
    to {
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .breakdown-label {
      animation: none;
    }
  }
</style>
