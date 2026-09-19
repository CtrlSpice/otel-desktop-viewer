<script lang="ts">
  import type { Attributes } from '@/types/api-types'
  import ConflictMarker from './ConflictMarker.svelte'
  import AttributeValueView from './AttributeValue.svelte'

  type Props = { attributes: Attributes; owner: string }
  let { attributes, owner }: Props = $props()
</script>

{#each attributes as attribute, index (`${owner}:${attribute.id ?? 'unidentified'}:${index}`)}
  <tr class="table-row">
    <td class="detail-cell">
      <span class="detail-cell__attribute-header">
        {#if attribute.hasConflict}
          <ConflictMarker
            label={`Conflicting typed values retained for ${attribute.key} in ${owner}.`}
          />
        {/if}
        <span class="detail-cell__key"
          >{attribute.key}
          <span class="detail-cell__type">({attribute.value.kind})</span>:</span
        >
      </span>
      <div class="detail-cell__attribute-value">
        <AttributeValueView
          value={attribute.value}
          path={`${owner}.${attribute.key}`}
        />
      </div>
    </td>
  </tr>
{/each}

<style lang="postcss">
  @reference "../../app.css";
  .detail-cell__type {
    color: var(--color-subtle);
    @apply font-normal;
  }
  .detail-cell__attribute-header {
    @apply flex max-w-full flex-wrap items-center gap-1;
  }
  .detail-cell__attribute-value {
    @apply mt-0.5 min-w-0;
  }
</style>
