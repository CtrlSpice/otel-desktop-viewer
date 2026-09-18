<script lang="ts">
  import type { Attributes } from '@/types/api-types'
  import AttributeValueView from './AttributeValue.svelte'

  type Props = { attributes: Attributes; owner: string }
  let { attributes, owner }: Props = $props()

  let conflicts = $derived([
    ...new Set(
      attributes.filter(attr => attr.hasConflict).map(attr => attr.key)
    ),
  ])
</script>

{#if conflicts.length > 0}
  <tr class="table-row">
    <td class="detail-cell">
      <p class="attribute-rows__conflict" role="alert">
        <span aria-label="Warning">&#9650;</span> Conflicting typed values for {conflicts.join(
          ', '
        )} in {owner}
      </p>
    </td>
  </tr>
{/if}
{#each attributes as attribute, index (`${owner}:${attribute.id ?? 'unidentified'}:${index}`)}
  <tr class="table-row">
    <td class="detail-cell">
      <span class="detail-cell__key"
        >{attribute.key}
        <span class="detail-cell__type">({attribute.value.kind})</span>:</span
      >
      <AttributeValueView
        value={attribute.value}
        path={`${owner}.${attribute.key}`}
      />
    </td>
  </tr>
{/each}

<style lang="postcss">
  @reference "../../app.css";
  .detail-cell__type {
    color: var(--color-subtle);
    @apply font-normal;
  }
  .attribute-rows__conflict {
    @apply m-0 text-xs text-warning;
  }
</style>
