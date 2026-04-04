<script lang="ts">
  import type { LinkData } from '@/types/api-types';
  import SpanField from './SpanField.svelte';
  import { router } from 'tinro5';

  type Props = {
    links: LinkData[];
  };

  let { links }: Props = $props();

  let openLinks = $state<Set<number>>(new Set([0]));

  function toggle(index: number, e: MouseEvent) {
    e.stopPropagation();
    let next = new Set(openLinks);
    if (next.has(index)) {
      next.delete(index);
    } else {
      next.add(index);
    }
    openLinks = next;
  }
</script>

{#each links as link, index}
  {@const open = openLinks.has(index)}
  <tr class="table-row">
    <th scope="row" class="table-cell--field-name">
      <div class="links-panel__field-name-inner">
        <button
          type="button"
          class="group-toggle"
          class:group-toggle--open={open}
          aria-expanded={open}
          aria-label={open ? `Collapse link ${index + 1}` : `Expand link ${index + 1}`}
          onclick={(e) => toggle(index, e)}
        >
          <svg
            class="group-toggle__caret"
            viewBox="0 0 24 24"
            fill="none"
            aria-hidden="true"
          >
            <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="1.5" />
            <path d="M10 8l4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>
        <span class="links-panel__key">traceID<span aria-hidden="true">:</span></span>
      </div>
      <span class="col-resize-marker" aria-hidden="true"></span>
    </th>
    <td class="table-cell">
      <div class="links-panel__value-cell">
        <a
          class="links-panel__trace-link"
          href="/trace/{link.traceID}"
          onclick={(e) => {
            e.preventDefault()
            e.stopPropagation()
            router.goto(`/trace/${link.traceID}`)
          }}
        >
          {link.traceID}
        </a>
        <span class="links-panel__badges">
          <span class="badge-type">string</span>
        </span>
      </div>
    </td>
  </tr>
  {#if open}
    <SpanField nested fieldName="span id" fieldValue={link.spanID} fieldType="string" />
    <SpanField nested fieldName="trace state" fieldValue={link.traceState} fieldType="string" />
    {#each link.attributes as attr}
      <SpanField
        nested
        fieldName={attr.key}
        fieldValue={attr.value}
        fieldType={attr.type}
      />
    {/each}
    {#if link.droppedAttributesCount > 0}
      <SpanField
        nested
        fieldName="dropped attributes count"
        fieldValue={link.droppedAttributesCount.toString()}
        fieldType="uint32"
      />
    {/if}
  {/if}
{/each}

<style lang="postcss">
  @reference "../../../app.css";
  .links-panel__field-name-inner {
    @apply flex min-w-0 items-center gap-1.5;
  }

  .links-panel__key {
    @apply min-w-0 truncate;
  }

  .links-panel__value-cell {
    @apply flex min-w-0 items-center gap-1.5;
  }

  .links-panel__trace-link {
    @apply min-w-0 flex-1 truncate font-mono text-xs text-primary underline decoration-primary/30 underline-offset-2;
    @apply hover:decoration-primary/60 hover:text-primary;
  }

  .links-panel__badges {
    @apply flex shrink-0 items-center gap-1;
  }
</style>
