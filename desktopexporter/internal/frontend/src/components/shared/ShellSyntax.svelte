<script lang="ts">
  import { onMount } from 'svelte'
  import { StyleModule } from 'style-mod'
  import { editorHighlightStyle } from './utils/editor-highlighting'
  import { shellTokens } from './utils/shell-highlighting'

  let { text }: { text: string } = $props()
  const tokens = $derived(shellTokens(text))
  onMount(() => {
    if (editorHighlightStyle.module)
      StyleModule.mount(document, editorHighlightStyle.module)
  })
</script>

<span class="font-mono text-sm"
  >{#each tokens as token, index (index)}<span class={token.classes}
      >{token.text}</span
    >{/each}</span
>
