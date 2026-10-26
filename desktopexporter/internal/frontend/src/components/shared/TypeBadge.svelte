<script lang="ts">
  let { type }: { type: string } = $props()
  let tone = $derived(
    type === 'string' || type === 'bytes'
      ? 'badge-accent'
      : /^(u?int|fixed|double|float)/.test(type)
        ? 'badge-warning'
        : type === 'empty'
          ? 'badge-neutral'
          : 'badge-primary'
  )
</script>

<span class="badge badge-xs shrink-0 {tone}">{type}</span>
