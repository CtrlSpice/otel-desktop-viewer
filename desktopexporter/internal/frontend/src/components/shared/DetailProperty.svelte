<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { AttributeValue } from '@/types/api-types'
  import ExpandableValue from './ExpandableValue.svelte'
  import Identifier from './Identifier.svelte'
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
    loadText,
    valueLabel,
    valueTone,
    valueColor,
    timestampLabel,
    qualifier,
    origin,
    multiline = false,
    hidden = false,
    nested = false,
    showType = true,
    defectLabel,
    colspan = 1,
    identifier = false,
    fieldKind = 'field',
  }: {
    fieldName: string
    fieldType: string
    fieldValue?: string
    typedValue?: AttributeValue
    value?: Snippet
    copyValue?: string
    loadText?: (signal: AbortSignal) => Promise<string>
    valueLabel?: string
    valueTone?: 'error'
    valueColor?: string
    timestampLabel?: string
    /** Subtle source context shown as `field · qualifier · type`. */
    qualifier?: string
    origin?: string
    multiline?: boolean
    hidden?: boolean
    nested?: boolean
    showType?: boolean
    defectLabel?: string
    colspan?: number
    identifier?: boolean
    fieldKind?: 'field' | 'attribute'
  } = $props()

  let conflict = $derived(
    defectLabel ??
      (typedValue && hasConflictingKeys(typedValue)
        ? `Conflicting typed values retained in ${fieldName}. Duplicate keys remain in the JSON output.`
        : undefined)
  )
  let copyText = $derived(
    copyValue ?? (typedValue ? attributeValueCopy(typedValue) : fieldValue)
  )
</script>

{#snippet header()}
  <div class="detail-property__header">
    {#if conflict}<ConflictMarker label={conflict} />{/if}
    <div class="detail-property__heading">
      <span class="detail-property__key"
        >{fieldName}{#if qualifier}<span class="text-rp-subtle">{' · '}</span
          ><span class="detail-property__qualifier">{qualifier}</span
          >{/if}{#if showType}<span class="text-rp-subtle">{' · '}</span><span
            class="detail-property__type">{fieldType}</span
          >{/if}</span
      >
      {#if origin}<span class="text-rp-subtle">{' · '}</span><span
          class="detail-property__origin">{origin}</span
        >{/if}
    </div>
    <ValueCopyButton
      text={copyText}
      {loadText}
      label={origin ? `${fieldName} (${origin})` : fieldName}
      contextual
    />
  </div>
{/snippet}

{#if !hidden}
  <tr
    class="detail-property"
    class:detail-property--error-value={valueTone === 'error'}
    class:table-row--nested={nested}
    style:--detail-property-value-color={valueColor}
    data-field={fieldName}
    data-field-source={origin}
    data-field-kind={fieldKind}
  >
    <td class="detail-property__cell" {colspan}>
      <div
        class="detail-property__card item-state"
        role="group"
        aria-label={fieldName}
      >
        {@render header()}
        <div class="detail-property__body">
          {#if valueTone === 'error'}
            <span class="detail-property__error-dot" aria-hidden="true"></span>
          {/if}
          {#if timestampLabel !== undefined}
            <div class="detail-property__value" title={fieldValue}>
              {#if value}{@render value()}{:else}{timestampLabel}{/if}
            </div>
            {#if /^[+-]?\d+(?:\.\d+)?$/.test(fieldValue.trim()) && fieldValue !== timestampLabel}
              <div class="detail-property__raw-timestamp">{fieldValue}</div>
            {/if}
          {:else if identifier && !value}
            <div class="detail-property__value">
              <Identifier value={fieldValue} />
            </div>
          {:else if !value && !valueLabel && !typedValue && fieldType === 'string'}
            <AttributeValueView
              value={{ kind: 'string', value: fieldValue }}
              path={fieldName}
              clamp={!multiline}
            />
          {:else if value || typedValue || multiline || valueLabel}
            <div class="detail-property__value">
              {#if value}{@render value()}
              {:else if typedValue}<AttributeValueView
                  value={typedValue}
                  path={fieldName}
                />
              {:else}{fieldValue}{#if valueLabel}<span
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
              value={fieldValue}
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

  .detail-property {
    scroll-margin-block: 3rem 0.5rem;
  }

  :global(.detail-property--revealed) .detail-property__card {
    --item-idle-bg: var(--color-highlight-low);
  }
  .detail-property__card {
    --item-idle-bg: var(--color-base-100);
    --json-editor-background: transparent;
    @apply min-w-0 rounded-xl py-2;
    padding-inline: var(--pane-inline-inset);
  }
  .detail-property__header {
    @apply flex min-h-5 min-w-0 items-center gap-2;
  }
  .detail-property__header > :global(.value-copy) {
    @apply ml-auto size-5 shrink-0 self-start;
  }
  .detail-property__heading {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .detail-property__key {
    @apply min-w-0 font-sans text-xs;
    color: var(--color-subtle);
    overflow-wrap: anywhere;
  }
  .detail-property__origin,
  .detail-property__type {
    color: var(--color-secondary);
  }
  .detail-property__qualifier {
    color: var(--color-subtle);
  }
  .detail-property__body {
    @apply min-w-0 pt-0.5;
  }
  .detail-property--error-value .detail-property__body {
    display: grid;
    grid-template-columns: 0.5rem minmax(0, 1fr);
    column-gap: 0.5rem;
  }
  .detail-property__error-dot {
    @apply size-2 rounded-full bg-error;
    margin-top: calc((1lh - 0.5rem) / 2);
  }
  .detail-property--error-value .detail-property__annotation {
    color: inherit;
  }
  .detail-property__value,
  :global(.detail-property .detail-property__value) {
    @apply min-w-0 font-sans text-base;
    color: var(--detail-property-value-color, var(--color-base-content));
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .detail-property__annotation {
    @apply ml-2 text-xs;
    color: var(--color-subtle);
  }
  .detail-property__annotation--enum {
    @apply ml-0;
  }
  .detail-property__raw-timestamp {
    @apply min-w-0 font-sans text-xs;
    color: var(--color-base-content);
    overflow-wrap: anywhere;
  }
</style>
