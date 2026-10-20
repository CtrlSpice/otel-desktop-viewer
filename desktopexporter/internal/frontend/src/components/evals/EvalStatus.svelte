<script lang="ts">
  let {
    label,
    tone,
    title,
  }: {
    label: string
    tone?: 'success' | 'warning' | 'error' | 'papaya'
    title?: string
  } = $props()
</script>

<span
  class="eval-status inline-flex min-w-0 items-center gap-2 text-sm font-normal text-base-content"
  {title}
>
  {#if tone}
    <span
      class="size-2 shrink-0 rounded-full"
      class:bg-success={tone === 'success'}
      class:bg-warning={tone === 'warning'}
      class:bg-error={tone === 'error'}
      class:bg-papaya={tone === 'papaya'}
      aria-hidden="true"
    ></span>
  {/if}
  <span>{label}</span>
</span>
