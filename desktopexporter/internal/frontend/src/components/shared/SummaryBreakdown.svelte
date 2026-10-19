<script lang="ts">
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
</script>

{#if colouredParts.length > 1}
  <div
    class="mt-2 mb-1 flex h-2 w-full rounded-full"
    role="img"
    aria-label={`Service breakdown by span count: ${colouredParts.map(part => `${part.label}: ${part.count} spans`).join('; ')}`}
  >
    {#each colouredParts as part (part.key)}
      <span
        class="service-segment tooltip tooltip-bottom min-w-0 first:rounded-l-full last:rounded-r-full"
        style:flex={part.count}
        style:background-color={part.color}
      >
        <span
          class="tooltip-content flex items-center gap-1.5 bg-base-200 text-sm font-normal text-base-content"
        >
          <span
            class="size-1.5 shrink-0 rounded-full"
            style:background-color={part.color}
            aria-hidden="true"
          ></span>
          <span>{part.label}</span>
          <span class="badge-count">{part.count.toLocaleString()} spans</span>
        </span>
      </span>
    {/each}
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .service-segment {
    --tt-bg: var(--color-base-200);
  }

  .service-segment:hover {
    z-index: 10;
  }

  .service-segment:first-child .tooltip-content {
    left: 0;
    transform: translateY(var(--tt-pos, 0.25rem));
  }

  .service-segment:last-child .tooltip-content {
    left: auto;
    right: 0;
    transform: translateY(var(--tt-pos, 0.25rem));
  }
</style>
