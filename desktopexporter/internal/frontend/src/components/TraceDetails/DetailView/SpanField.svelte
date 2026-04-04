<script lang="ts">
  type Props = {
    fieldType: string
    fieldName: string
    fieldValue: string
    origin?: 'resource' | 'scope'
    hidden?: boolean
    isRoot?: boolean
    /** Align with one waterfall gutter column under expand rows (events / links). */
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
    <th scope="row" class="table-cell--field-name">
      {fieldName}<span aria-hidden="true">:</span>
      <span class="col-resize-marker" aria-hidden="true"></span>
    </th>
    <td class="table-cell">
      <div class="span-field__value-cell">
        <span class="span-field__value-text">{fieldValue}</span>
        <span class="span-field__badges">
          <span class="badge-type">{fieldType}</span>
          {#if origin}
            <span class="badge-origin">{origin}</span>
          {/if}
          {#if isRoot}
            <span class="badge-root">root</span>
          {/if}
        </span>
      </div>
    </td>
  </tr>
{/if}

<style lang="postcss">
  @reference "../../../app.css";
  .span-field__value-cell {
    @apply flex min-w-0 items-center gap-1.5;
  }

  .span-field__value-text {
    @apply min-w-0 flex-1 truncate;
  }

  .span-field__badges {
    @apply flex shrink-0 items-center gap-1;
  }
</style>
