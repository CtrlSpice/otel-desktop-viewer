<script lang="ts">
  import type { LinkData } from '@/types/api-types'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import SpanField from './SpanField.svelte'
  import { itemHref, navigateToItem } from '@/route'
  import { SPAN_PARAM } from '@/route/query-params'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import Alert02Icon from '@hugeicons/core-free-icons/Alert02Icon'

  type Props = {
    links: LinkData[]
  }

  let { links }: Props = $props()

  function linkFieldCount(link: LinkData): number {
    // trace id, span id, trace state -- the three rows that always render.
    // This read 2 and undercounted by one for every link ever shown; the
    // trace-state row was never accounted for.
    let n = 3
    n += link.attributes.length
    if (link.droppedAttributesCount > 0) n++
    if (link.flags > 0) n++
    return n
  }

  function spanPatch(link: LinkData) {
    return link.spanID ? { [SPAN_PARAM]: link.spanID } : undefined
  }

  function goToLink(e: MouseEvent, link: LinkData) {
    e.preventDefault()
    if (!link.traceID) return
    navigateToItem('traces', link.traceID, 'push', spanPatch(link))
  }
</script>

{#snippet invalidLinkIcon()}
  <span
    class="links-panel__warning"
    title="Invalid link target"
    aria-hidden="true"
  >
    <HugeiconsIcon icon={Alert02Icon} size="1em" strokeWidth={1.5} />
  </span>
{/snippet}

{#each links as link, index (index)}
  {@const patch = spanPatch(link)}
  {@const href = link.traceID ? itemHref('traces', link.traceID, patch) : null}
  {@const invalid = !link.traceID || !link.spanID}
  <FieldGroup
    label={link.traceID ?? 'Invalid link target'}
    icon={invalid ? invalidLinkIcon : undefined}
    count={linkFieldCount(link)}
    open={index === 0}
  >
    <table
      class="detail-fields w-full"
      aria-label="Link {link.traceID ?? 'invalid target'}"
    >
      <tbody>
        <tr class="table-row">
          <td class="detail-cell">
            <span class="detail-cell__key">
              trace id <span class="detail-cell__type">(string | null)</span>:
            </span>
            {#if href}
              <a
                class="detail-cell__value link link-primary font-mono"
                {href}
                onclick={e => goToLink(e, link)}>{link.traceID}</a
              >
            {:else}
              <span class="detail-cell__value font-mono">null</span>
            {/if}
          </td>
        </tr>
        <tr class="table-row">
          <td class="detail-cell">
            <span class="detail-cell__key">
              span id <span class="detail-cell__type">(string | null)</span>:
            </span>
            {#if href && link.spanID}
              <a
                class="detail-cell__value link link-primary font-mono"
                {href}
                onclick={e => goToLink(e, link)}>{link.spanID}</a
              >
            {:else}
              <span class="detail-cell__value font-mono"
                >{link.spanID ?? 'null'}</span
              >
            {/if}
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
        {#if link.flags > 0}
          <SpanField
            fieldName="flags"
            fieldValue={link.flags.toString()}
            fieldType="uint32"
          />
        {/if}
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

  .links-panel__warning {
    @apply text-warning inline-flex flex-none text-sm;
  }
</style>
