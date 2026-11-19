<script lang="ts">
  // ExpandableValue takes its key as a snippet, which render() cannot pass
  // directly; this stands in for a field component.
  import ExpandableValue from '@/components/shared/Detail/ExpandableValue.svelte'

  let { value, identity }: { value: string; identity?: unknown } = $props()
</script>

{#snippet keyLabel()}
  <span class="detail-cell__key">field:</span>
{/snippet}

<ExpandableValue {keyLabel} {value} {identity} />
