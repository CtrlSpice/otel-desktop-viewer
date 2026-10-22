<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'

  let {
    storageKey,
    label,
    layoutKey,
  }: { storageKey: string; label: string; layoutKey?: string } = $props()
</script>

<DetailStack label="Test details" {storageKey}>
  <DetailBubble {label} {layoutKey}>Selected value</DetailBubble>
  <DetailBubble label="Telemetry">Telemetry value</DetailBubble>
  <DetailBubble label="Configuration" initialOpen={false}
    >Configuration value</DetailBubble
  >
</DetailStack>
