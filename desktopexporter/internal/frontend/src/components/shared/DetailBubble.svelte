<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    label,
    icon,
    count,
    countLabel = label.toLowerCase(),
    children,
  }: {
    label: string
    icon?: Snippet
    count?: number
    countLabel?: string
    children: Snippet
  } = $props()
</script>

<details class="collapse min-h-0 overflow-y-auto bg-base-100" open>
  <summary
    class="collapse-title sticky top-0 z-10 flex items-center gap-2 bg-base-100 pe-4 text-sm font-medium"
  >
    {@render icon?.()}
    <span>{label}</span>
    {#if count !== undefined}
      <span class="badge-count ml-auto shrink-0">{count} {countLabel}</span>
    {/if}
  </summary>
  <div class="collapse-content">
    {@render children()}
  </div>
</details>
