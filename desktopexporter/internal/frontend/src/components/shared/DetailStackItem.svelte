<script lang="ts">
  import { setContext, type Snippet } from 'svelte'
  import { DETAIL_SECTION } from './utils/detail-stack-state.svelte'

  let { id, content }: { id: string; content: Snippet } = $props()
  setContext(DETAIL_SECTION, () => id)
</script>

{@render content()}
