<script lang="ts">
  // ExpandableValue takes its content as a snippet, which a test cannot pass
  // directly to render(); this stands in for a field component.
  import ExpandableValue from '@/components/shared/ExpandableValue.svelte'

  let { value }: { value: string } = $props()
</script>

<ExpandableValue resetKey={value}>
  <span class="detail-cell__key">field:</span>
  <span class="detail-cell__value">{value}</span>
</ExpandableValue>
