<script lang="ts">
  import ExpandableValue from '@/components/shared/ExpandableValue.svelte'
  import type { Snippet } from 'svelte'

  type Props = {
    fieldType: string
    fieldName: string
    fieldValue?: string
    /** Custom value cell (links, etc.). Omit when using fieldValue. */
    value?: Snippet
    multiline?: boolean
    hidden?: boolean
    /** Show `· type` after the field name. Defaults to true. */
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
  <tr class="table-row">
    <td class="detail-cell" class:detail-field__cell--multiline={multiline}>
      {#snippet keyLabel()}
        <span class="detail-cell__key">
          {fieldName}{#if showType}
            <span class="text-subtle">{' · '}</span><span
              class="detail-cell__type">{fieldType}</span
            >{/if}
        </span>
      {/snippet}
      <ExpandableValue {keyLabel} value={fieldValue ?? ''}>
        {#if value}
          <div class="detail-field__value">{@render value()}</div>
        {:else if multiline}
          <span
            class="detail-cell__value detail-field__value detail-field__value--multiline"
            class:tabular-nums={fieldType === 'timestamp'}>{fieldValue}</span
          >
        {:else}
          <div class:tabular-nums={fieldType === 'timestamp'}>
            {fieldValue ?? ''}
          </div>
        {/if}
      </ExpandableValue>
    </td>
  </tr>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .detail-cell__type {
    color: var(--color-secondary);
    @apply font-normal;
  }

  .detail-field__cell--multiline {
    @apply align-top whitespace-normal;
    max-width: none;
  }

  .detail-field__value {
    @apply mt-0.5 min-w-0;
  }
  .detail-field__value--multiline {
    @apply block text-base-content;
    white-space: pre-wrap;
    overflow-wrap: break-word;
    word-break: break-word;
  }
</style>
