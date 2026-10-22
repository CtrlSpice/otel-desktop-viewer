<script lang="ts">
  import type { AttributeValue } from '@/types/api-types'
  import { attributeValueTokens, jsonContainerTokens } from './detail-value'
  import ValueSyntax from './ValueSyntax.svelte'
  import JsonEditor from './JsonEditor.svelte'

  let { value, path }: { value: AttributeValue; path?: string } = $props()
  let json = $derived(
    value.kind === 'string' ? jsonContainerTokens(value.value, true) : null
  )
  let readable = $derived(
    value.kind === 'string' && value.value.length > 0 && !json
  )
  let tokens = $derived(json ?? attributeValueTokens(value, true))
  let structured = $derived(
    Boolean(json) || value.kind === 'array' || value.kind === 'map'
  )
</script>

<div
  class="attribute-value"
  class:attribute-value--text={readable}
  aria-label={path ? `Value of ${path}` : undefined}
>
  {#if readable && value.kind === 'string'}
    {value.value}
  {:else if structured}
    <JsonEditor
      text={tokens.map(token => token.text).join('')}
      label={path ? `JSON value of ${path}` : 'JSON value'}
    />
  {:else}
    <pre><code><ValueSyntax {tokens} highlight={false} /></code></pre>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";
  .attribute-value {
    @apply m-0 min-w-0 font-sans text-base;
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
  .attribute-value code {
    font: inherit;
  }
  .attribute-value--text {
    @apply font-sans text-sm;
  }
</style>
