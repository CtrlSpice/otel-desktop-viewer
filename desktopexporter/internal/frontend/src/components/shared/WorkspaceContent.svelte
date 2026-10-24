<script lang="ts">
  import { setContext, untrack, type Snippet } from 'svelte'

  let {
    contexts,
    children,
  }: {
    contexts: Map<unknown, unknown>
    children: Snippet
  } = $props()

  // A slot mounts outside its route's component ancestry. Carry that route's
  // contexts with its content, without replacing the surrounding pane.
  for (const [key, value] of untrack(() => contexts)) setContext(key, value)
</script>

{@render children()}
