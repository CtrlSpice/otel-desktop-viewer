<script lang="ts">
  import type { AttributeValue } from '@/types/api-types'
  import {
    attributeValueTokens,
    isReadableText,
    jsonContainerTokens,
  } from './detail-value'
  import ExpandableValue from './ExpandableValue.svelte'

  let {
    value,
    path,
    clamp = false,
  }: { value: AttributeValue; path?: string; clamp?: boolean } = $props()
  let json = $derived(
    value.kind === 'string' ? jsonContainerTokens(value.value) : null
  )
  let readable = $derived(
    value.kind === 'string' && !json && isReadableText(value.value, path)
  )
  let tokens = $derived(json ?? attributeValueTokens(value))
  let structured = $derived(
    Boolean(json) || value.kind === 'array' || value.kind === 'map'
  )
</script>

<div
  class="attribute-value"
  class:attribute-value--text={readable}
  class:attribute-value--structured={structured}
  aria-label={path ? `Value of ${path}` : undefined}
>
  {#if readable && value.kind === 'string'}
    {#if clamp}<ExpandableValue value={value.value} />{:else}{value.value}{/if}
  {:else}
    <pre><code
        >{#each tokens as token, index (index)}<span
            class="detail-code--{token.kind}"
            title={token.type}>{token.text}</span
          >{/each}</code
      ></pre>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";
  .attribute-value {
    @apply m-0 min-w-0 font-mono text-[0.8125rem] leading-relaxed;
    color: var(--detail-property-value-color, var(--color-base-content));
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    tab-size: 2;
  }
  .attribute-value pre {
    margin: 0;
    font: inherit;
    white-space: inherit;
  }
  .attribute-value--text {
    @apply font-sans text-sm;
  }
  .attribute-value :global(.detail-pair__value) {
    color: inherit;
  }
  .attribute-value--structured {
    @apply mt-1 border-l border-base-content/15 py-1 pl-3 text-xs;
  }
  .attribute-value--structured .detail-code--key {
    color: var(--detail-code-key);
  }
  .attribute-value--structured .detail-code--string {
    color: var(--detail-code-string);
  }
  .attribute-value--structured .detail-code--number {
    color: var(--detail-code-number);
  }
  .attribute-value--structured .detail-code--literal {
    color: var(--detail-code-literal);
  }
  .attribute-value--structured .detail-code--punctuation {
    color: var(--detail-code-key);
  }
</style>
