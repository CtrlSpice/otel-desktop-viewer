<script lang="ts">
  import ExpandableValue from '@/components/shared/ExpandableValue.svelte'
  import ConflictMarker from '@/components/shared/ConflictMarker.svelte'
  type Props = {
    fieldType: string
    fieldName: string
    fieldValue: string
    defectLabel?: string
    hidden?: boolean
    nested?: boolean
  }

  let {
    fieldType,
    fieldName,
    fieldValue,
    defectLabel,
    hidden = false,
    nested = false,
  }: Props = $props()
</script>

{#if !hidden}
  <tr class="table-row" class:table-row--nested={nested}>
    <td class="detail-cell">
      {#snippet keyLabel()}
        <span class="detail-cell__field-header">
          {#if defectLabel}<ConflictMarker label={defectLabel} />{/if}
          <span class="detail-cell__key">
            {fieldName}
            <span class="detail-cell__type">({fieldType})</span>:
          </span>
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

  .detail-cell__field-header {
    @apply flex max-w-full flex-wrap items-center gap-1;
  }
</style>
