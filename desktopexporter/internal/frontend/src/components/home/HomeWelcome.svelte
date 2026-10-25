<script lang="ts">
  import type { Snippet } from 'svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import lulu from '@/assets/images/lulu.webp'

  let {
    importContent,
    sections = [],
  }: {
    importContent?: Snippet
    sections?: { id: string; content: Snippet }[]
  } = $props()
  const taglines = [
    'OpenTelemetry made cozy',
    "I'm not changing the name, buddy.",
    'Now with “a lot more functionality”™',
    "I have splintered the ecosystem. Pray I don't splinter it any further.",
    // [1] README.md references: https://doi.org/10.1038/nature25458
    'An axolotl’s genome is about ten times the size of a human’s.',
    // [2] README.md references: https://doi.org/10.1073/pnas.1300290110
    'Axolotls need immune cells called macrophages to regrow a limb.',
    // [3] README.md references: https://doi.org/10.1126/science.abp9262
    'Axolotls can replace lost neurons and rebuild connections in parts of their forebrain.',
    // [4] README.md references: https://doi.org/10.1186/1741-7007-10-103
    'Axolotls lose the ability to regrow an eye lens about two weeks after hatching.',
  ]
  const tagline = taglines[Math.floor(Math.random() * taglines.length)]
</script>

<div class="home-welcome">
  <div class="welcome-banner grid shrink-0 items-center gap-4 py-2">
    <img
      src={lulu}
      alt="Lulu the axolotl with her telescope"
      class="aspect-square min-w-0 w-full object-contain"
    />
    <div class="flex min-w-0 flex-col gap-3">
      <h2
        class="welcome-title flex items-baseline gap-2 font-mono font-normal text-primary"
      >
        <span class="welcome-prompt shrink-0 select-none" aria-hidden="true"
          >&gt;</span
        >
        <span>otel-desktop-viewer</span>
      </h2>
      <p class="text-balance text-subtle">{tagline}</p>
    </div>
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
  }
  .welcome-banner {
    grid-template-columns: minmax(0, min(32%, 7rem)) minmax(min-content, 1fr);
  }
  .welcome-title {
    @apply whitespace-nowrap text-lg;
    line-height: 1.25;
  }
  .welcome-prompt {
    color: var(--color-code-sora);
  }
  .home-import {
    @apply shrink-0;
  }
</style>
