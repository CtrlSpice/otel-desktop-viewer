<script lang="ts">
  let { label, count = 0 }: { label: string; count?: number } = $props()
</script>

<output>{label}: {count}</output>
