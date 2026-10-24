<script lang="ts">
  import type { ValueToken } from './detail-value'
  import { splitJsonString } from './utils/json-escapes'

  let {
    tokens,
    highlight = true,
  }: {
    tokens: ValueToken[]
    highlight?: boolean
  } = $props()
</script>

<span class:value-syntax={highlight}
  >{#each tokens as token, index (index)}<span
      class="detail-code--{token.kind}"
      title={token.type}
      >{#if highlight && (token.kind === 'key' || token.kind === 'string')}{#each splitJsonString(token.text) as part}<span
            class:detail-code--escape={part.kind === 'escape'}
            class:detail-code--quote={part.kind === 'quote'}>{part.text}</span
          >{/each}{:else}{token.text}{/if}</span
    >{/each}</span
>
