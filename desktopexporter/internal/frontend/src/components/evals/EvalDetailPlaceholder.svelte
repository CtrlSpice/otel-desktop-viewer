<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
</script>

{#snippet details()}<DetailBubble label="Details"
    ><p class="p-3 text-subtle">
      Select an experiment to view details
    </p></DetailBubble
  >{/snippet}
{#snippet telemetry()}<DetailBubble label="Telemetry" disabled
    ><span></span></DetailBubble
  >{/snippet}
<DetailStack
  label="Evaluation details"
  storageKey="eval-detail-section-sizes"
  sections={[
    { id: 'details', content: details },
    { id: 'telemetry', content: telemetry },
  ]}
/>
