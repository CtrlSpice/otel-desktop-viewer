<script lang="ts">
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'

  let {
    storageKey,
    label,
    layoutKey,
    assessments = false,
    unavailable = false,
  }: {
    storageKey: string
    label: string
    layoutKey?: string
    assessments?: boolean
    unavailable?: boolean
  } = $props()
</script>

{#snippet detailsSection()}
  <DetailBubble {label} {layoutKey}>Selected value</DetailBubble>
{/snippet}
{#snippet assessmentsSection()}
  <DetailBubble label="Assessments">Assessment value</DetailBubble>
{/snippet}
{#snippet telemetrySection()}
  <DetailBubble label="Telemetry">Telemetry value</DetailBubble>
{/snippet}
{#snippet configurationSection()}
  <DetailBubble label="Configuration" initialOpen={false}
    >Configuration value</DetailBubble
  >
{/snippet}
{#snippet inactiveSection()}
  <DetailBubble label="Unavailable" disabled>Unavailable value</DetailBubble>
{/snippet}
<DetailStack
  label="Test details"
  {storageKey}
  sections={[
    { id: 'details', content: detailsSection },
    ...(assessments
      ? [{ id: 'assessments', content: assessmentsSection }]
      : []),
    ...(unavailable ? [{ id: 'unavailable', content: inactiveSection }] : []),
    { id: 'telemetry', content: telemetrySection },
    { id: 'configuration', content: configurationSection },
  ]}
/>
