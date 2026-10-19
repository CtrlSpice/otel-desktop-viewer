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
    class="mt-2 flex h-2 w-full overflow-hidden rounded-full"
    role="img"
    aria-label={`Service breakdown by span count: ${colouredParts.map(part => `${part.label}: ${part.count} spans`).join('; ')}`}
  >
    {#each colouredParts as part (part.key)}
      <span
        style:flex={part.count}
        style:background-color={part.color}
        title={`${part.label}: ${part.count} spans`}
      ></span>
    {/each}
  </div>
  <ul class="service-grid" aria-label="Spans by service">
    {#each colouredParts as part (part.key)}
      <li
        class="service-grid__item"
        title={`${part.label}: ${part.count} spans${part.key === rootKey ? ' · root service' : ''}`}
      >
        <span
          class="size-1.5 shrink-0 rounded-full"
          style:background-color={part.color}
          aria-hidden="true"
        ></span>
        <span class="min-w-0 truncate">{part.label}</span>
        <span class="ml-auto shrink-0 tabular-nums"
          >{part.count.toLocaleString()}<span class="sr-only">
            spans</span
          ></span
        >
      </li>
    {/each}
  </ul>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .service-grid {
    @apply m-0 grid min-w-0 list-none gap-x-3 gap-y-1 pt-1 pb-0.5 text-sm font-normal leading-tight;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 9rem), 1fr));
    color: var(--color-subtle);
  }

  .service-grid__item {
    @apply flex min-w-0 items-center gap-1.5;
  }
</style>
