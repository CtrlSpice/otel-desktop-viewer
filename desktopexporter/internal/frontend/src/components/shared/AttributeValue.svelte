<script lang="ts">
  import ChevronDownIcon from '@hugeicons/core-free-icons/ChevronDownIcon'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import type { AttributeMapEntry, AttributeValue } from '@/types/api-types'
  import AttributeValueView from './AttributeValue.svelte'

  type ChildLabel =
    { kind: 'index'; value: number } | { kind: 'key'; value: string }
  type ScalarValue = Exclude<AttributeValue, { kind: 'array' | 'map' }>

  type Props = {
    value: AttributeValue
    path?: string
    label?: ChildLabel
    trailingComma?: boolean
  }

  let { value, path = '', label, trailingComma = false }: Props = $props()
  let expanded = $state(false)

  let itemCount = $derived(
    value.kind === 'array' || value.kind === 'map' ? value.value.length : 0
  )
  let countLabel = $derived(
    `${itemCount} ${value.kind === 'map' ? (itemCount === 1 ? 'entry' : 'entries') : itemCount === 1 ? 'item' : 'items'}`
  )
  let containsConflicts = $derived(containsConflict(value))

  function formatScalar(value: ScalarValue): string {
    switch (value.kind) {
      case 'empty':
        return 'null'
      case 'string':
        return JSON.stringify(value.value)
      case 'bytes':
        return value.value
      case 'bool':
      case 'int64':
        return String(value.value)
      case 'double':
        if (Object.is(value.value, -0)) return '-0'
        if (Number.isNaN(value.value)) return 'NaN'
        if (value.value === Infinity) return 'Infinity'
        if (value.value === -Infinity) return '-Infinity'
        return String(value.value)
    }
  }

  function formatLabel(label: ChildLabel): string {
    return label.kind === 'index'
      ? `[${label.value}]`
      : JSON.stringify(label.value)
  }

  function mapEntries(value: AttributeValue): AttributeMapEntry[] {
    if (value.kind !== 'map') throw new Error('Expected an attribute map')
    return value.value
  }

  function containsConflict(value: AttributeValue): boolean {
    switch (value.kind) {
      case 'map':
        return (
          (value.conflictingKeys?.length ?? 0) > 0 ||
          value.value.some(entry => containsConflict(entry.value))
        )
      case 'array':
        return value.value.some(containsConflict)
      default:
        return false
    }
  }
</script>

{#snippet rowPrefix()}
  {#if label}
    <span class="attribute-value__caret-spacer" aria-hidden="true"></span>
    <span class="attribute-value__label">{formatLabel(label)}:</span>
  {/if}
{/snippet}

{#if value.kind !== 'array' && value.kind !== 'map'}
  <span
    class:attribute-value__row={label !== undefined}
    class="attribute-value__scalar"
  >
    {@render rowPrefix()}
    <span class="attribute-value__value">{formatScalar(value)}</span>
    {#if label}
      <span class="attribute-value__kind">({value.kind})</span>
    {/if}
    {#if trailingComma}<span class="attribute-value__comma">,</span>{/if}
  </span>
{:else if itemCount === 0}
  <span
    class:attribute-value__row={label !== undefined}
    class="attribute-value__empty-container"
  >
    {@render rowPrefix()}
    <span class="attribute-value__punctuation"
      >{value.kind === 'array' ? '[]' : '{}'}</span
    >
    <span
      class="badge-count"
      class:badge-warning={containsConflicts}
      aria-label={containsConflicts
        ? `${countLabel}; contains conflicting typed values`
        : countLabel}>{countLabel}</span
    >
    {#if trailingComma}<span class="attribute-value__comma">,</span>{/if}
  </span>
{:else}
  <details class="attribute-value__container" bind:open={expanded}>
    <summary class="attribute-value__summary">
      <HugeiconsIcon
        icon={ChevronDownIcon}
        size="1em"
        strokeWidth={1.5}
        class="attribute-value__caret"
        aria-hidden="true"
      />
      {#if label}
        <span class="attribute-value__label">{formatLabel(label)}:</span>
      {/if}
      <span class="attribute-value__punctuation"
        >{expanded
          ? value.kind === 'array'
            ? '['
            : '{'
          : value.kind === 'array'
            ? '[…]'
            : '{…}'}</span
      >
      {#if !expanded && trailingComma}<span class="attribute-value__comma"
          >,</span
        >{/if}
      <span
        class="badge-count"
        class:badge-warning={containsConflicts}
        aria-label={containsConflicts
          ? `${countLabel}; contains conflicting typed values`
          : countLabel}>{countLabel}</span
      >
    </summary>
    {#if expanded}
      <div class="attribute-value__children">
        {#if value.kind === 'array'}
          {#each value.value as child, index (`${path}[${index}]`)}
            <div
              class="attribute-value__child"
              class:attribute-value__child--conflicted={containsConflict(child)}
            >
              <AttributeValueView
                value={child}
                path={`${path}[${index}]`}
                label={{ kind: 'index', value: index }}
                trailingComma={index < itemCount - 1}
              />
            </div>
          {/each}
        {:else}
          {@const entries = mapEntries(value)}
          {#each entries as entry, index (`${path}.${entry.key}:${index}`)}
            {@const isConflicted =
              containsConflict(entry.value) ||
              value.conflictingKeys?.includes(entry.key)}
            <div
              class="attribute-value__child"
              class:attribute-value__child--conflicted={isConflicted}
            >
              <AttributeValueView
                value={entry.value}
                path={`${path}.${entry.key}`}
                label={{ kind: 'key', value: entry.key }}
                trailingComma={index < itemCount - 1}
              />
            </div>
          {/each}
        {/if}
      </div>
      <div class="attribute-value__closing">
        <span class="attribute-value__caret-spacer" aria-hidden="true"></span>
        {value.kind === 'array' ? ']' : '}'}{#if trailingComma}<span
            class="attribute-value__comma">,</span
          >{/if}
      </div>
    {/if}
  </details>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .attribute-value__scalar {
    @apply font-mono text-xs text-base-content;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .attribute-value__value {
    @apply min-w-0;
    overflow-wrap: anywhere;
  }
  .attribute-value__label,
  .attribute-value__kind {
    @apply shrink-0 whitespace-nowrap;
  }
  .attribute-value__label {
    @apply font-mono;
    color: var(--color-subtle);
  }
  .attribute-value__kind {
    color: var(--color-subtle);
  }
  .attribute-value__container {
    @apply text-xs;
  }
  .attribute-value__summary {
    @apply flex cursor-pointer list-none items-center gap-1 font-mono;
    color: var(--color-subtle);
  }
  .attribute-value__summary::marker,
  .attribute-value__summary::-webkit-details-marker {
    display: none;
  }
  .attribute-value__caret-spacer,
  .attribute-value__summary :global(.attribute-value__caret) {
    @apply inline-block shrink-0 align-middle;
    width: var(--fg-caret-size, 0.875rem);
    height: var(--fg-caret-size, 0.875rem);
  }
  .attribute-value__summary :global(.attribute-value__caret) {
    @apply transition-transform duration-150;
    color: var(--color-muted);
    transform: rotate(-90deg);
  }
  .attribute-value__container[open]
    > .attribute-value__summary
    :global(.attribute-value__caret) {
    transform: rotate(0deg);
  }
  .attribute-value__children {
    --attribute-indent: calc(var(--fg-caret-size, 0.875rem) + 0.5rem);
    @apply relative grid gap-1 py-1;
    padding-inline-start: var(--attribute-indent);
  }
  .attribute-value__child {
    @apply relative min-w-0;
  }
  .attribute-value__child::before {
    @apply pointer-events-none absolute -top-1 -bottom-1 w-px bg-base-content/15;
    content: '';
    left: calc(
      -1 * var(--attribute-indent) + var(--fg-caret-size, 0.875rem) / 2
    );
  }
  .attribute-value__child--conflicted::before {
    @apply bg-warning;
  }
  .attribute-value__row {
    @apply flex min-w-0 items-baseline gap-1;
  }
  .attribute-value__comma,
  .attribute-value__closing,
  .attribute-value__punctuation {
    color: var(--color-subtle);
  }
  .attribute-value__closing {
    @apply flex items-baseline gap-1 font-mono text-xs;
  }
  .attribute-value__empty-container {
    @apply font-mono text-xs items-baseline gap-1;
    color: var(--color-subtle);
  }
</style>
