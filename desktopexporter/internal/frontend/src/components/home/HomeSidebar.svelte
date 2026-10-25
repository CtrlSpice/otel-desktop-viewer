<script lang="ts">
  import type { Snippet } from 'svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'

  let {
    sections = [],
  }: {
    sections?: { id: string; content: Snippet }[]
  } = $props()
</script>

<div class="home-sidebar">
  <DetailStack
    label="Home sections"
    storageKey="home-getting-started"
    defaultWeights={{
      'Getting started': 0.7,
      Storage: 0.15,
      'Ingestion issues': 0.15,
    }}
    {sections}
  />
</div>

<style lang="postcss">
  @reference '../../app.css';
  .home-sidebar {
    @apply flex h-full min-h-0 min-w-0 flex-col pb-3;
    padding-inline-start: var(--layout-gap);
  }
</style>
