<script lang="ts">
  import type { LinkData } from '@/types/api-types'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import SpanField from './SpanField.svelte'
  import { navigateToItem } from '@/utils/url-state'

  type Props = {
    links: LinkData[]
  }

  let { links }: Props = $props()

  function linkFieldCount(link: LinkData): number {
    let n = 2
    n += link.attributes.length
    if (link.droppedAttributesCount > 0) n++
    return n
  }
</script>

{#each links as link, index (index)}
  <FieldGroup label={link.traceID} count={linkFieldCount(link)} open={index === 0}>
    <table class="detail-fields w-full" aria-label="Link {link.traceID}">
      <tbody>
        <tr class="table-row">
          <td class="detail-cell">
            <span class="detail-cell__key">
              trace id <span class="detail-cell__type">(string)</span>:
            </span>
            <a
              class="detail-cell__value link link-primary font-mono"
              href="/traces/{link.traceID}"
              onclick={e => {
                e.preventDefault()
                navigateToItem('traces', link.traceID)
              }}
            >{link.traceID}</a>
          </td>
        </tr>
        <SpanField fieldName="span id" fieldValue={link.spanID} fieldType="string" />
        <SpanField
          fieldName="trace state"
          fieldValue={link.traceState}
          fieldType="string"
        />
        {#each link.attributes as attr (attr.key)}
          <SpanField
            fieldName={attr.key}
            fieldValue={attr.value}
            fieldType={attr.type}
          />
        {/each}
        {#if link.droppedAttributesCount > 0}
          <SpanField
            fieldName="dropped attributes count"
            fieldValue={link.droppedAttributesCount.toString()}
            fieldType="uint32"
          />
        {/if}
      </tbody>
    </table>
  </FieldGroup>
{/each}

<style lang="postcss">
  @reference "../../../app.css";

  .detail-cell__type {
    color: var(--color-subtle);
    @apply font-normal;
  }
</style>
