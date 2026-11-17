<script lang="ts">
  let { status, label }: { status: string; label: string } = $props()
</script>

<span class="badge badge-soft result-badge result-{status}">{label}</span>
