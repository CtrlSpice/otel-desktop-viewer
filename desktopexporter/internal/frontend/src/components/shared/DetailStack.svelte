<script lang="ts">
  import type { Snippet } from 'svelte'

  let { label, children }: { label: string; children: Snippet } = $props()
</script>

<div
  class="grid min-h-0 flex-1 auto-rows-[minmax(0,auto)] content-start gap-2 overflow-hidden"
  role="region"
  aria-label={label}
>
  {@render children()}
</div>
