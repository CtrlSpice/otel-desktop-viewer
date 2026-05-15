<script lang="ts">
  // Mirror of SpanField (TraceDetails/DetailView/SpanField.svelte).
  // Same DOM shape -- <tr class="table-row"> with detail-cell + badges --
  // so metric and span detail tables visually agree without sharing a
  // component across signal types. Local copy keeps each signal free to
  // evolve its row vocabulary (e.g. metric-only origins like
  // 'datapoint') without dragging the others along.
  type Props = {
    fieldType: string
    fieldName: string
    fieldValue: string
    // 'resource' / 'scope' come straight from OTLP. 'datapoint' is for
    // attrs that live on an individual datapoint -- not used yet by the
    // metric-level table but reserved so the badge palette stays
    // consistent if/when we surface them here.
    origin?: 'resource' | 'scope' | 'datapoint'
    hidden?: boolean
    nested?: boolean
  }

  let {
    fieldType,
    fieldName,
    fieldValue,
    origin,
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
  @reference "../../app.css";

  .detail-cell__type {
    color: var(--color-subtle);
    @apply font-normal;
  }
</style>
