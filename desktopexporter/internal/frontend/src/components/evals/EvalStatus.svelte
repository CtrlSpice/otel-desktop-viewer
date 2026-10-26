<script lang="ts">
  let {
    label,
    tone,
    title,
    dotOnly = false,
  }: {
    label: string
    tone?:
      | 'success'
      | 'complete'
      | 'running'
      | 'warning'
      | 'error'
      | 'papaya'
      | 'neutral'
    title?: string
    dotOnly?: boolean
  } = $props()
</script>

<span
  class="eval-status inline-flex min-w-0 items-center gap-2 text-sm font-normal text-base-content"
  class:shrink-0={dotOnly}
  title={title ?? (dotOnly ? label : undefined)}
  role={dotOnly ? 'img' : undefined}
  aria-label={dotOnly ? label : undefined}
>
  {#if tone}
    <span
      class="size-2 shrink-0 rounded-full"
      class:bg-accent={tone === 'success'}
      class:bg-secondary={tone === 'complete'}
      class:bg-primary={tone === 'running'}
      class:bg-warning={tone === 'warning'}
      class:bg-error={tone === 'error'}
      class:bg-papaya={tone === 'papaya'}
      class:bg-base-300={tone === 'neutral'}
      aria-hidden="true"
    ></span>
  {/if}
  {#if !dotOnly}<span>{label}</span>{/if}
</span>
