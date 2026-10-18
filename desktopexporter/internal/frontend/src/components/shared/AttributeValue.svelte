<script lang="ts">
  import type { AttributeValue } from '@/types/api-types'
  import { attributeValueTokens } from './detail-value'

  let { value, path }: { value: AttributeValue; path?: string } = $props()
  let tokens = $derived(attributeValueTokens(value))
  let structured = $derived(value.kind === 'array' || value.kind === 'map')
</script>

<pre
  class="attribute-value"
  class:attribute-value--structured={structured}
  aria-label={path ? `Value of ${path}` : undefined}><code
    >{#each tokens as token, index (index)}<span
        class="detail-code--{token.kind}"
        title={token.type}>{token.text}</span
      >{/each}</code
  ></pre>

<style lang="postcss">
  @reference "../../app.css";
  .attribute-value {
    @apply m-0 min-w-0 font-mono text-[0.8125rem] leading-relaxed;
    color: var(--color-base-content);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    tab-size: 2;
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
