<script lang="ts">
  import type { Snippet } from 'svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import Lulu from './Lulu.svelte'

  let {
    importContent,
    sections = [],
  }: {
    importContent?: Snippet
    sections?: { id: string; content: Snippet }[]
  } = $props()
</script>

<div class="home-welcome">
  <div class="welcome-banner">
    <div class="welcome-lulu"><Lulu /></div>
    <h2 class="welcome-title font-mono font-normal text-primary">
      <span>otel-desktop-viewer</span>
    </h2>
  </div>
  {#if importContent}
    <section class="home-import" aria-label="Import telemetry">
      {@render importContent()}
    </section>
  {/if}
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
  .home-welcome {
    @apply flex h-full min-h-0 min-w-0 flex-col gap-3 p-3;
    padding-top: 0;
  }
  .welcome-banner {
    @apply grid shrink-0 items-center gap-x-6;
    grid-template-columns: minmax(0, min(32%, 6rem)) minmax(min-content, 1fr);
    grid-template-rows: calc(2 * var(--pane-header-row-h));
    padding-block: var(--header-padding);
  }
  .welcome-lulu {
    @apply w-full min-w-0;
    height: calc(100% + 2 * var(--header-padding));
  }
  .welcome-title {
    @apply whitespace-nowrap text-lg;
    line-height: var(--text-lg--line-height);
  }
  .home-import {
    @apply shrink-0;
  }
</style>
