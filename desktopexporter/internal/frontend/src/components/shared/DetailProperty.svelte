<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { AttributeValue } from '@/types/api-types'
  import ExpandableValue from './ExpandableValue.svelte'
  import Identifier from './Identifier.svelte'
  import AttributeValueView from './AttributeValue.svelte'
  import ConflictMarker from './ConflictMarker.svelte'
  import ValueCopyButton from './ValueCopyButton.svelte'
  import JsonEditor from './JsonEditor.svelte'
  import ShellValue from './ShellValue.svelte'
  import {
    valuePresentation,
    valuePresentationKey,
  } from '@/state/value-presentation.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { UnfoldLessIcon, UnfoldMoreIcon } from '@hugeicons/core-free-icons'
  import {
    attributeValueCopy,
    hasConflictingKeys,
    structuredValueText,
  } from './detail-value'

  let {
    fieldName,
    fieldType,
    fieldValue = '',
    typedValue,
    jsonText,
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
    fieldSource,
  }: {
    fieldName: string
    fieldType: string
    fieldValue?: string
    typedValue?: AttributeValue
    /** Already-formatted JSON for an explicitly structured source value. */
    jsonText?: string
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
    fieldSource?: string
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
  let structuredText = $derived(
    jsonText ??
      (!value && !valueLabel && timestampLabel === undefined && !identifier
        ? typedValue
          ? structuredValueText(typedValue)
          : fieldType === 'string'
            ? structuredValueText({ kind: 'string', value: fieldValue })
            : undefined
        : undefined)
  )
  let editor = $state<{ collapseAll: () => void; expandAll: () => void }>()
  const presentationKey = $derived(
    valuePresentationKey(fieldSource ?? origin ?? 'field', fieldKind, fieldName)
  )
  const shellText = $derived(
    !value &&
      !identifier &&
      timestampLabel === undefined &&
      !valueLabel &&
      !jsonText
      ? typedValue?.kind === 'string'
        ? typedValue.value
        : !typedValue && fieldType === 'string'
          ? fieldValue
          : undefined
      : undefined
  )
  const showShell = $derived(
    shellText !== undefined && valuePresentation.isShell(presentationKey)
  )
</script>

{#snippet header()}
  <div class="detail-property__header">
    {#if conflict}<ConflictMarker label={conflict} />{/if}
    <div class="detail-property__heading">
      <span class="detail-property__key"
        >{fieldName}{#if qualifier}<span class="text-subtle">{' · '}</span><span
            class="detail-property__qualifier">{qualifier}</span
          >{/if}{#if showType}<span class="text-subtle">{' · '}</span><span
            class="detail-property__type">{fieldType}</span
          >{/if}</span
      >
      {#if origin}<span class="text-subtle">{' · '}</span><span
          class="detail-property__origin">{origin}</span
        >{/if}
    </div>
    {#if structuredText !== undefined && !showShell}
      <div
        class="detail-property__folding"
        role="group"
        aria-label={`JSON folding for ${fieldName}`}
      >
        <button
          type="button"
          class="header-icon-button tooltip tooltip-left"
          aria-label={`Collapse all ${fieldName} JSON`}
          data-tip="Collapse all"
          onclick={() => editor?.collapseAll()}
        >
          <HugeiconsIcon
            icon={UnfoldLessIcon}
            size={16}
            strokeWidth={1.5}
            aria-hidden="true"
          />
        </button>
        <button
          type="button"
          class="header-icon-button tooltip tooltip-left"
          aria-label={`Expand all ${fieldName} JSON`}
          data-tip="Expand all"
          onclick={() => editor?.expandAll()}
        >
          <HugeiconsIcon
            icon={UnfoldMoreIcon}
            size={16}
            strokeWidth={1.5}
            aria-hidden="true"
          />
        </button>
      </div>
    {/if}
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
          {#if showShell && shellText !== undefined}
            <ShellValue text={shellText} label={fieldName} />
          {:else if structuredText !== undefined}
            <JsonEditor
              text={structuredText}
              label={`JSON value of ${fieldName}`}
              defaultOpenDepth={1}
              bind:this={editor}
            />
          {:else}
            <ExpandableValue value={copyText}>
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
                <div class="detail-property__value">{fieldValue}</div>
              {/if}
            </ExpandableValue>
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
  .detail-property__folding {
    @apply ml-auto flex shrink-0 self-start gap-1;
  }
  .detail-property__folding + :global(.value-copy) {
    margin-left: 0;
  }
  .detail-property__heading {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .detail-property__key {
    @apply min-w-0 font-sans text-xs;
    color: var(--color-primary);
    overflow-wrap: anywhere;
  }
  .detail-property__origin,
  .detail-property__type {
    color: var(--color-subtle);
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
