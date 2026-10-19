<script lang="ts">
  let { value }: { value: string } = $props()
  // Trace IDs and UUIDs retain eight characters at each end; shorter IDs retain four.
  let edge = $derived(value.length >= 32 ? 8 : 4)
  let display = $derived(
    value.length > edge * 2 + 1
      ? `${value.slice(0, edge)}…${value.slice(-edge)}`
      : value
  )
</script>

<span class="identifier" title={value}>
  {#if display === value}{value}{:else}<span class="sr-only">{value}</span><span
      aria-hidden="true">{display}</span
    >{/if}
</span>

<style>
  .identifier {
    white-space: nowrap;
  }
</style>
