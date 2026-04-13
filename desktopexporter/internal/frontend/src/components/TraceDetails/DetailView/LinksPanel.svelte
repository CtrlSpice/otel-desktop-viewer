<script lang="ts">
  import { tick } from 'svelte'
  import type { LinkData } from '@/types/api-types'
  import SpanField from './SpanField.svelte'
  import { router } from 'tinro5'
  import { ArrowDownIcon } from '@/icons'

  type Props = {
    links: LinkData[]
  }

  let { links }: Props = $props()

  let expandedIndex = $state<number | null>(0)
  let headerRows: HTMLTableRowElement[] = []

  function toggle(index: number) {
    const opening = expandedIndex !== index
    expandedIndex = opening ? index : null
    if (opening) {
      tick().then(() => {
        headerRows[index]?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
      })
    }
  }
</script>

{#each links as link, index}
  {@const open = expandedIndex === index}
  <tr
    bind:this={headerRows[index]}
    class="table-row links-panel__header-row cursor-pointer {open ? 'links-panel__header-row--open' : ''}"
    onclick={() => toggle(index)}
    role="button"
    tabindex="0"
    onkeydown={e => (e.key === 'Enter' || e.key === ' ') && toggle(index)}
  >
    <td class="detail-cell">
      <span class="links-panel__indicator {open ? 'links-panel__indicator--open' : ''}">
        <ArrowDownIcon />
      </span>
      <span class="detail-cell__key">traceID:</span>
      <a
        class="link link-primary font-mono"
        href="/trace/{link.traceID}"
        onclick={e => {
          e.preventDefault()
          e.stopPropagation()
          router.goto(`/trace/${link.traceID}`)
        }}
      >{link.traceID}</a>
    </td>
    <td class="detail-cell--badges">
      <span class="badge-type">string</span>
    </td>
  </tr>
  {#if open}
    <SpanField
      nested
      fieldName="span id"
      fieldValue={link.spanID}
      fieldType="string"
    />
    <SpanField
      nested
      fieldName="trace state"
      fieldValue={link.traceState}
      fieldType="string"
    />
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

  .links-panel__header-row--open,
  .links-panel__header-row--open ~ :global(.table-row--nested) {
    @apply bg-base-200/40;
  }

  .links-panel__indicator {
    @apply inline-flex align-middle text-base-content/35 transition-all duration-150 mr-1;
    font-size: 14px;
    transform: rotate(-90deg);
  }

  .links-panel__indicator--open {
    @apply text-base-content/70;
    transform: rotate(0deg);
  }

  .links-panel__header-row:hover .links-panel__indicator {
    @apply text-base-content/60;
  }
</style>
