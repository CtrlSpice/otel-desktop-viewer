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
</script>

{#if colouredParts.length > 1}
  <div
    class="mt-2 flex h-2 w-full overflow-hidden rounded-full"
    role="img"
    aria-label={`Service breakdown by span count: ${colouredParts.map(part => `${part.label}: ${part.count} spans`).join('; ')}`}
  >
    {#each colouredParts as part (part.key)}
      <span
        class="service-segment min-w-0"
        style:flex={part.count}
        style:--service-color={part.color}
      ></span>
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
    <span
      class="shrink-0 text-rp-subtle tabular-nums"
      aria-label={`${part.count} spans`}>({part.count.toLocaleString()})</span
    >
  </span>
{/snippet}

{#if colouredParts.length > 1}
  <div
    class="service-disclosure"
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
    {#each colouredParts as part (part.key)}
      <button
        type="button"
        class="service-choice"
        class:service-choice--open={activeKey === part.key}
        data-service-key={part.key}
        aria-label={`${part.label}: ${part.count} spans`}
        aria-expanded={activeKey === part.key}
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
      >
        {@render serviceLabel(part)}
      </button>
    {/each}
  </div>
{:else if colouredParts.length === 1}
  <div class="service-single">
    {@render serviceLabel(colouredParts[0])}
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .service-segment {
    background-color: color-mix(
      in srgb,
      var(--service-color) calc(var(--waterfall-bar-opacity) * 100%),
      transparent
    );
  }

  .service-disclosure {
    @apply mt-1 flex h-7 min-w-0 items-center gap-1;
  }

  .service-choice {
    @apply flex h-7 min-w-6 cursor-pointer items-center gap-1.5 overflow-hidden rounded-sm px-2 text-left text-sm font-normal text-base-content;
    flex: 0 0 1.5rem;
    transition: flex-grow 240ms cubic-bezier(0.2, 0, 0.2, 1);
  }

  .service-choice--open {
    flex-grow: 1;
  }

  .service-choice:focus-visible {
    outline: 2px solid var(--color-primary);
    outline-offset: -2px;
  }

  .service-dot {
    @apply size-2 shrink-0 rounded-full;
  }

  .service-label {
    @apply flex min-w-0 items-center gap-1.5 overflow-hidden whitespace-nowrap;
  }

  .service-choice .service-label {
    opacity: 0;
    transition: opacity 160ms ease-out;
  }

  .service-choice--open .service-label {
    opacity: 1;
  }

  .service-single {
    @apply mt-1 flex h-7 min-w-0 items-center gap-1.5 text-sm font-normal text-base-content;
  }

  @media (prefers-reduced-motion: reduce) {
    .service-choice,
    .service-choice .service-label {
      transition: none;
    }
  }
</style>
