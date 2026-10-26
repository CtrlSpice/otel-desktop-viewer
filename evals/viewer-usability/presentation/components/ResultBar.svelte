<script lang="ts">
  import Badge from './Badge.svelte'
  import type { outcomeBreakdown } from './model'

  let {
    parts,
    label,
    task = false,
  }: {
    parts: ReturnType<typeof outcomeBreakdown>
    label: string
    task?: boolean
  } = $props()
</script>

<div
  class="grade-strip"
  role="img"
  aria-label="{label}: {parts.map(part => part.text).join('; ') ||
    'No results'}"
>
  {#each parts as part}
    <span class="result-{part.status}" style="flex:{part.count}"></span>
  {/each}
</div>
<div class="grade-counts" class:task-result={task}>
  {#each parts as part}
    <Badge status={part.status} label={part.text} />
  {:else}
    No results
  {/each}
</div>
