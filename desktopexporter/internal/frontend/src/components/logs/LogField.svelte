<script lang="ts">
  import type { Snippet } from 'svelte'

  type Props = {
    fieldType: string
    fieldName: string
    fieldValue?: string
    /** Custom value cell (links, etc.). Omit when using fieldValue. */
    value?: Snippet
    multiline?: boolean
    hidden?: boolean
    /** Show `(type)` after the field name. Defaults to true. */
    showType?: boolean
  }

  let {
    fieldType,
    fieldName,
    fieldValue,
    value,
    multiline = false,
    hidden = false,
    showType = true,
  }: Props = $props()
</script>

{#if !hidden}
  <tr class="table-row" class:log-field--multiline={multiline}>
    <td class="detail-cell" class:log-field__cell--multiline={multiline}>
      <span class="detail-cell__key">
        {fieldName}{#if showType}
          {' '}<span class="detail-cell__type">({fieldType})</span>{/if}:
      </span>
      {#if value}
        {@render value()}
      {:else}
        <span
          class="detail-cell__value"
          class:log-field__value--multiline={multiline}
          class:tabular-nums={fieldType === 'timestamp'}
        >{fieldValue}</span>
      {/if}
    </td>
  </tr>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .detail-cell__type {
    color: var(--color-subtle);
    @apply font-normal;
  }

  .log-field__cell--multiline {
    @apply align-top whitespace-normal;
    max-width: none;
  }

  .log-field__value--multiline {
    @apply mt-0.5 block text-base-content;
    white-space: pre-wrap;
    overflow-wrap: break-word;
    word-break: break-word;
  }
</style>
