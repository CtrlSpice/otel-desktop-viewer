<script lang="ts">
  import type { AttributeMapEntry, AttributeValue } from '@/types/api-types'
  import AttributeValueView from './AttributeValue.svelte'

  type Props = {
    value: AttributeValue
    path?: string
  }

  let { value, path = '' }: Props = $props()

  let conflicts = $derived(
    value.kind === 'map' ? (value.conflictingKeys ?? []) : []
  )

  function mapEntries(value: AttributeValue): AttributeMapEntry[] {
    return value.kind === 'map' ? value.value : []
  }
</script>

{#if value.kind === 'empty'}
  <span class="attribute-value__scalar">null</span>
{:else if value.kind === 'string' || value.kind === 'bytes'}
  <span class="attribute-value__scalar">{value.value}</span>
{:else if value.kind === 'bool' || value.kind === 'int64' || value.kind === 'double'}
  <span class="attribute-value__scalar">{String(value.value)}</span>
{:else if value.kind === 'array'}
  <ol class="attribute-value__list">
    {#each value.value as child, index (`${path}[${index}]`)}
      <li><AttributeValueView value={child} path={`${path}[${index}]`} /></li>
    {/each}
  </ol>
{:else}
  {#if conflicts.length > 0}
    <p class="attribute-value__conflict" role="alert">
      <span aria-label="Warning">&#9650;</span> Conflicting values for {conflicts.join(
        ', '
      )} at {path || 'map'}
    </p>
  {/if}
  {@const entries = mapEntries(value)}
  <dl class="attribute-value__map">
    {#each entries as entry, index (`${path}.${entry.key}:${index}`)}
      <div>
        <dt>{entry.key}</dt>
        <dd>
          <AttributeValueView
            value={entry.value}
            path={`${path}.${entry.key}`}
          />
        </dd>
      </div>
    {/each}
  </dl>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .attribute-value__scalar {
    @apply font-mono text-xs;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .attribute-value__list,
  .attribute-value__map {
    @apply m-0 mt-1 pl-4 text-xs;
  }
  .attribute-value__map > div {
    @apply flex gap-2;
  }
  .attribute-value__map dt {
    @apply shrink-0 font-mono;
    color: var(--color-subtle);
  }
  .attribute-value__map dd {
    @apply m-0 min-w-0;
  }
  .attribute-value__conflict {
    @apply m-0 mb-1 text-xs text-warning;
  }
</style>
