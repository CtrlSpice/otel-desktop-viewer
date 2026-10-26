<script lang="ts">
  let {
    pending,
    label = 'Loading…',
    requestKey,
  }: {
    pending: boolean
    label?: string
    requestKey?: string | null
  } = $props()

  let visible = $state(false)
  $effect(() => {
    void requestKey
    visible = false
    if (!pending) return
    const timer = setTimeout(() => (visible = true), 200)
    return () => clearTimeout(timer)
  })
</script>

{#if pending && visible}
  <span class="text-sm text-subtle" role="status">{label}</span>
{/if}
