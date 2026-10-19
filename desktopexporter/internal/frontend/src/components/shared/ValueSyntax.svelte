<script lang="ts">
  import type { ValueToken } from './detail-value'

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
      title={token.type}>{token.text}</span
    >{/each}</span
>
