<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'

  let {
    storageKey,
    label,
    layoutKey,
    assessments = false,
  }: {
    storageKey: string
    label: string
    layoutKey?: string
    assessments?: boolean
  } = $props()
</script>

<DetailStack label="Test details" {storageKey}>
  <DetailBubble {label} {layoutKey}>Selected value</DetailBubble>
  {#if assessments}
    <DetailBubble label="Assessments">Assessment value</DetailBubble>
  {/if}
  <DetailBubble label="Telemetry">Telemetry value</DetailBubble>
  <DetailBubble label="Configuration" initialOpen={false}
    >Configuration value</DetailBubble
  >
</DetailStack>
