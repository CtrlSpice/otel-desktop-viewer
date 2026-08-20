<script lang="ts">
  import ExpandableValue from '@/components/shared/ExpandableValue.svelte'
  type Props = {
    fieldType: string
    fieldName: string
    fieldValue: string
    hidden?: boolean
    isRoot?: boolean
    nested?: boolean
  }

  let {
    fieldType,
    fieldName,
    fieldValue,
    hidden = false,
    isRoot = false,
    nested = false,
  }: Props = $props()
</script>

{#if !hidden}
  <tr class="table-row" class:table-row--nested={nested}>
    <td class="detail-cell">
      {#snippet keyLabel()}
        <span class="detail-cell__key">
          {fieldName}
          <span class="detail-cell__type">({fieldType})</span>{#if isRoot}<span
              class="detail-cell__type"
            >
              (root)</span
            >{/if}:
        </span>
      {/snippet}
      <ExpandableValue {keyLabel} value={fieldValue} />
    </td>
  </tr>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .detail-cell__type {
    color: var(--color-subtle);
    @apply font-normal;
  }
</style>
