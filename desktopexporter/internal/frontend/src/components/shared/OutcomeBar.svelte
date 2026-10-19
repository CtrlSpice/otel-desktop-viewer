<script module lang="ts">
  export type OutcomePart = {
    label: string
    count: number
    tone: 'success' | 'warning' | 'error' | 'papaya' | 'neutral'
  }
</script>

<script lang="ts">
  let { parts, label }: { parts: OutcomePart[]; label: string } = $props()
  const total = $derived(parts.reduce((sum, part) => sum + part.count, 0))
  const populated = $derived(parts.filter(part => part.count > 0))
  function text(part: OutcomePart): string {
    return `${part.count} ${part.label} · ${Number(((part.count / total) * 100).toFixed(1))}%`
  }
</script>

<div
  class="flex h-2 w-full overflow-hidden rounded-full bg-base-300"
  role="img"
  aria-label={`${label}: ${populated.map(text).join('; ') || 'No results'}`}
>
  {#each populated as part}
    <span
      title={text(part)}
      class:bg-success={part.tone === 'success'}
      class:bg-warning={part.tone === 'warning'}
      class:bg-error={part.tone === 'error'}
      class:bg-papaya={part.tone === 'papaya'}
      class:bg-neutral={part.tone === 'neutral'}
      style:flex={part.count}
    ></span>
  {/each}
</div>
