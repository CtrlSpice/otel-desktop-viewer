<script lang="ts">
  // Mirror of SpanField (TraceDetails/DetailView/SpanField.svelte).
  // Same DOM shape -- <tr class="table-row"> with detail-cell + badges --
  // so metric and span detail tables visually agree without sharing a
  // component across signal types. Local copy keeps each signal free to
  // evolve its row vocabulary without dragging the others along.
  type Props = {
    fieldType: string
    fieldName: string
    fieldValue: string
    hidden?: boolean
    nested?: boolean
  }

  let {
    fieldType,
    fieldName,
    fieldValue,
    hidden = false,
    nested = false,
  }: Props = $props()
</script>

{#if !hidden}
  <tr class="table-row" class:table-row--nested={nested}>
    <td class="detail-cell">
      <span class="detail-cell__key">{fieldName} <span class="detail-cell__type">({fieldType})</span>:</span>
      <span class="detail-cell__value">{fieldValue}</span>
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
