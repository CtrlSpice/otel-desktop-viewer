<script lang="ts">
  type Props = {
    fieldType: string
    fieldName: string
    fieldValue: string
    origin?: 'resource' | 'scope'
    hidden?: boolean
    isRoot?: boolean
    nested?: boolean
  }

  let {
    fieldType,
    fieldName,
    fieldValue,
    origin,
    hidden = false,
    isRoot = false,
    nested = false,
  }: Props = $props()
</script>

{#if !hidden}
  <tr class="table-row" class:table-row--nested={nested}>
    <td class="detail-cell">
      <span class="detail-cell__key">{fieldName}:</span>
      <span class="detail-cell__value">{fieldValue}</span>
    </td>
    <td class="detail-cell--badges">
      <span class="badge-type">{fieldType}</span>
      {#if origin}
        <span class="badge-origin">{origin}</span>
      {/if}
      {#if isRoot}
        <span class="badge-root">root</span>
      {/if}
    </td>
  </tr>
{/if}

<style lang="postcss">
  @reference "../../../app.css";
</style>
