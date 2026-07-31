<script lang="ts">
  import type { LinkData } from '@/types/api-types'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import SpanField from './SpanField.svelte'
  import { itemHref, navigateToItem } from '@/route'
  import { SPAN_PARAM } from '@/route/query-params'

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

  function spanPatch(link: LinkData) {
    return link.spanID ? { [SPAN_PARAM]: link.spanID } : undefined
  }

  function goToLink(e: MouseEvent, link: LinkData) {
    e.preventDefault()
    navigateToItem('traces', link.traceID, 'push', spanPatch(link))
  }
</script>

{#each links as link, index (index)}
  {@const patch = spanPatch(link)}
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
              href={itemHref('traces', link.traceID, patch)}
              onclick={e => goToLink(e, link)}
            >{link.traceID}</a>
          </td>
        </tr>
        <tr class="table-row">
          <td class="detail-cell">
            <span class="detail-cell__key">
              span id <span class="detail-cell__type">(string)</span>:
            </span>
            <a
              class="detail-cell__value link link-primary font-mono"
              href={itemHref('traces', link.traceID, patch)}
              onclick={e => goToLink(e, link)}
            >{link.spanID}</a>
          </td>
        </tr>
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
