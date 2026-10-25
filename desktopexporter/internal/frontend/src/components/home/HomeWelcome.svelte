<script lang="ts">
  import type { Snippet } from 'svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import Lulu from './Lulu.svelte'

  let {
    sections = [],
  }: {
    sections?: { id: string; content: Snippet }[]
  } = $props()
</script>

<div class="home-welcome">
  <div class="welcome-banner">
    <div class="welcome-lulu"><Lulu /></div>
    <h2 class="welcome-title font-mono font-semibold text-primary">
      <span class="select-none text-info" aria-hidden="true">&gt;</span>
      <span>otel-desktop-viewer</span>
    </h2>
  </div>
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
    @apply flex h-full min-h-0 min-w-0 flex-col gap-3 pb-3;
    padding-inline-start: var(--layout-gap);
  }
  .welcome-banner {
    @apply flex shrink-0 items-center gap-3 rounded-xl bg-base-200;
    height: var(--pane-header-single-h);
    padding: var(--header-padding) var(--header-padding-inline);
  }
  .welcome-lulu {
    @apply size-8 shrink-0;
  }
  .welcome-title {
    @apply flex items-center gap-2 whitespace-nowrap text-lg;
    line-height: var(--text-lg--line-height);
  }
</style>
