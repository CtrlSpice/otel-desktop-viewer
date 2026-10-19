<script lang="ts">
  import { onDestroy } from 'svelte'
  import type { SummaryBreakdownPart } from '@/types/api-types'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'

  let {
    parts,
    rootService,
  }: { parts: SummaryBreakdownPart[]; rootService?: string } = $props()
  const rootKey = $derived(
    rootService === undefined ? undefined : `service:${rootService}`
  )
  const colouredParts = $derived(
    parts
      .filter(part => part.count > 0)
      .map(part => ({
        ...part,
        color: part.other
          ? 'var(--color-neutral)'
          : traceServiceColor(
              part.key.slice('service:'.length),
              themeSignal.value
            ),
      }))
      .sort((a, b) => Number(b.key === rootKey) - Number(a.key === rootKey))
  )
  let pinnedKey = $state<string | null>(null)
  let hoveredKey = $state<string | null>(null)
  let focusedKey = $state<string | null>(null)
  let hoverTimer: ReturnType<typeof setTimeout> | undefined
  let hoverTarget: string | null = null

  function cancelHoverPreview() {
    clearTimeout(hoverTimer)
    hoverTarget = null
  }

  function previewService(key: string) {
    if (hoverTarget === key) return
    cancelHoverPreview()
    hoverTarget = key
    hoverTimer = setTimeout(() => {
      hoveredKey = key
    }, 120)
  }

  onDestroy(cancelHoverPreview)
  const activeKey = $derived(
    [hoveredKey, focusedKey, pinnedKey, rootKey].find(key =>
      colouredParts.some(part => part.key === key)
    ) ?? colouredParts[0]?.key
  )
  const keptKey = $derived(
    colouredParts.find(part => part.key === pinnedKey)?.key ??
      colouredParts.find(part => part.key === rootKey)?.key ??
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
  function spanCount(part: SummaryBreakdownPart): string {
    return `${part.count.toLocaleString()} ${part.count === 1 ? 'span' : 'spans'}`
  }
</script>

<div
  class="flow-root min-w-0"
  role="group"
  aria-label="Spans by service"
  onpointermove={event => {
    if (event.pointerType === 'touch') return
    const target = event.target
    if (!(target instanceof Element)) return
    const choice = target.closest<HTMLElement>('[data-service-key]')
    if (choice?.dataset.serviceKey) previewService(choice.dataset.serviceKey)
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
      aria-label={`Service breakdown by span count: ${colouredParts.map(part => `${part.label}: ${spanCount(part)}`).join('; ')}`}
    >
      {#each colouredParts as part (part.key)}
        <button
          type="button"
          class="service-segment min-w-2 rounded-full"
          data-service-key={part.key}
          style:flex={part.count}
          style:--service-color={part.color}
          aria-label={`${part.label}: ${spanCount(part)}, ${percent.format(part.count / total)} of total`}
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

  {#snippet serviceLabel(part: (typeof colouredParts)[number])}
    <span
      class="service-dot"
      style:background-color={part.color}
      aria-hidden="true"
    ></span>
    <span class="service-label">
      <span class="min-w-0 truncate">{part.label}</span>
      <span class="shrink-0 text-rp-subtle tabular-nums"
        >({spanCount(part)}{#if colouredParts.length > 1},
          {percent.format(part.count / total)}{/if})</span
      >
    </span>
  {/snippet}

  {#if activePart}
    <div class="ball-pit">
      {#key activeKey}
        <div class="service-summary">
          {@render serviceLabel(activePart)}
        </div>
      {/key}
    </div>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .service-segment {
    @apply cursor-pointer border-0 p-0;
    background-color: color-mix(
      in srgb,
      var(--service-color) calc(var(--waterfall-bar-opacity) * 100%),
      transparent
    );
  }

  .service-segment:focus-visible {
    outline: 2px solid var(--color-primary);
    outline-offset: -2px;
  }

  :global(html[data-theme='rose-pine-dawn']) .service-segment {
    border: 1px solid var(--service-color);
  }

  .ball-pit {
    @apply mt-1 flex h-7 min-w-0 items-center;
  }

  .service-summary {
    @apply flex min-w-0 items-center gap-1.5 text-sm font-normal text-base-content;
    animation: service-label-in 140ms ease-out;
  }

  .service-dot {
    @apply size-2 shrink-0 rounded-full;
  }

  .service-label {
    @apply flex min-w-0 items-center gap-1.5 overflow-hidden whitespace-nowrap;
  }

  @keyframes service-label-in {
    from {
      opacity: 0.5;
    }
    to {
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .service-summary {
      animation: none;
    }
  }
</style>
