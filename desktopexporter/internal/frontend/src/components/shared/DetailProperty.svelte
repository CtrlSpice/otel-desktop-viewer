<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { AttributeValue } from '@/types/api-types'
  import ExpandableValue from './ExpandableValue.svelte'
  import AttributeValueView from './AttributeValue.svelte'
  import ConflictMarker from './ConflictMarker.svelte'
  import ValueCopyButton from './ValueCopyButton.svelte'
  import { attributeValueCopy, hasConflictingKeys } from './detail-value'

  let {
    fieldName,
    fieldType,
    fieldValue = '',
    typedValue,
    value,
    copyValue,
    valueLabel,
    valueTone,
    timestampLabel,
    origin,
    multiline = false,
    hidden = false,
    nested = false,
    showType = true,
    defectLabel,
    colspan = 1,
  }: {
    fieldName: string
    fieldType: string
    fieldValue?: string
    typedValue?: AttributeValue
    value?: Snippet
    copyValue?: string
    valueLabel?: string
    valueTone?: 'error'
    timestampLabel?: string
    origin?: string
    multiline?: boolean
    hidden?: boolean
    nested?: boolean
    showType?: boolean
    defectLabel?: string
    colspan?: number
  } = $props()

  let conflict = $derived(
    defectLabel ??
      (typedValue && hasConflictingKeys(typedValue)
        ? `Conflicting typed values retained in ${fieldName}. Duplicate keys remain in the JSON output.`
        : undefined)
  )
  let scalarText = $derived(
    fieldType === 'string' && !multiline
      ? JSON.stringify(fieldValue)
      : fieldValue
  )
  let copyText = $derived(
    copyValue ?? (typedValue ? attributeValueCopy(typedValue) : fieldValue)
  )
  let typeTone = $derived(
    fieldType === 'string' || fieldType === 'bytes'
      ? 'badge-accent'
      : /^(u?int|fixed|double|float)/.test(fieldType)
        ? 'badge-warning'
        : fieldType === 'empty'
          ? 'badge-neutral'
          : 'badge-primary'
  )
  let copyControl = $state<{ copy: () => Promise<void> }>()

  function copyCard(event: PointerEvent) {
    if (event.button !== 0 || !event.isPrimary) return
    const target = event.target
    if (!(target instanceof Element)) return
    if (
      target.closest(
        'a, button, input, select, textarea, summary, [contenteditable]'
      )
    )
      return
    // Selecting text is not a request to replace the clipboard with the whole value.
    if (window.getSelection()?.isCollapsed === false) return
    void copyControl?.copy()
  }
</script>

{#snippet header()}
  <div class="detail-property__header">
    {#if conflict}<ConflictMarker label={conflict} />{/if}
    <span class="detail-property__key">{fieldName}</span>
    <span class="detail-property__actions">
      {#if showType || origin}
        <span class="detail-property__badges">
          {#if showType}<span
              class="badge badge-xs badge-soft {typeTone} detail-property__type"
              >{fieldType}</span
            >{/if}
          {#if origin}<span
              class="badge badge-xs badge-soft badge-primary detail-property__origin"
              >{origin}</span
            >{/if}
        </span>
      {/if}
      <ValueCopyButton
        bind:this={copyControl}
        text={copyText}
        label={origin ? `${fieldName} (${origin})` : fieldName}
        contextual
      />
    </span>
  </div>
{/snippet}

{#if !hidden}
  <tr
    class="detail-property"
    class:detail-property--error-value={valueTone === 'error'}
    class:table-row--nested={nested}
    data-field={fieldName}
  >
    <td class="detail-property__cell" {colspan}>
      <div
        class="detail-property__card item-state"
        role="group"
        aria-label={fieldName}
        onpointerup={copyCard}
      >
        {@render header()}
        {#if timestampLabel !== undefined}
          <div class="detail-property__value detail-property__timestamp">
            {#if value}{@render value()}{:else}{timestampLabel}{/if}
          </div>
        {/if}
        <div class="detail-property__body">
          {#if timestampLabel !== undefined}
            <div class="detail-property__raw-timestamp">{fieldValue}</div>
          {:else if value || typedValue || multiline || valueLabel}
            <div class="detail-property__value">
              {#if value}{@render value()}
              {:else if typedValue}<AttributeValueView value={typedValue} />
              {:else}{scalarText}{#if valueLabel}<span
                    class="detail-property__annotation"
                    class:detail-property__annotation--enum={fieldType ===
                      'enum'}
                    >{fieldType === 'enum'
                      ? ` (${valueLabel})`
                      : valueLabel}</span
                  >{/if}{/if}
            </div>
          {:else}
            <ExpandableValue
              value={scalarText}
              valueClass="detail-property__value"
            />
          {/if}
        </div>
      </div>
    </td>
  </tr>
{/if}

<style lang="postcss">
  @reference "../../app.css";
  .detail-property__cell {
    @apply p-0 pb-1 align-top text-sm;
    max-width: 0;
  }
  .detail-property__card {
    --item-idle-bg: var(--color-base-100);
    @apply min-w-0 cursor-pointer rounded-xl px-3 py-2;
  }
  .detail-property__header {
    @apply flex min-h-7 min-w-0 items-center gap-2;
  }
  .detail-property__key {
    @apply min-w-0 font-mono text-xs;
    color: var(--detail-code-key);
    overflow-wrap: anywhere;
  }
  .detail-property__actions {
    @apply ml-auto flex shrink-0 items-center gap-2;
  }
  .detail-property__badges {
    @apply flex shrink-0 items-center justify-end gap-2;
  }
  .detail-property__body {
    @apply min-w-0 pt-0.5;
  }
  .detail-property--error-value {
    --detail-property-value-color: var(--color-error);
  }
  .detail-property__value,
  :global(.detail-property .detail-property__value) {
    @apply min-w-0 font-mono text-[0.8125rem] leading-relaxed;
    color: var(--detail-property-value-color, var(--color-base-content));
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .detail-property__annotation {
    @apply ml-2 text-xs;
    color: var(--detail-property-value-color, var(--detail-code-key));
  }
  .detail-property__annotation--enum {
    @apply ml-0;
  }
  .detail-property__timestamp {
    @apply pt-0.5;
  }
  .detail-property__raw-timestamp {
    @apply min-w-0 font-mono text-xs leading-5;
    color: var(--color-base-content);
    overflow-wrap: anywhere;
  }
</style>
