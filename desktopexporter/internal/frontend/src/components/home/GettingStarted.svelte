<script lang="ts">
  import type { Snippet } from 'svelte'
  import { onMount } from 'svelte'
  import PaneHeader, { paneTabID } from '@/components/shared/PaneHeader.svelte'
  import ReadonlyCodePanel from '@/components/shared/ReadonlyCodePanel.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import lulu from '@/assets/images/lulu.webp'
  import { QuestionMarkIcon } from '@/icons'
  import { setupAnchorPopover } from '@/components/shared/utils/anchor-popover'

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
  let protocol = $state('http')
  let httpPort = $state<number | null>(null)
  const helpID = $props.id()
  let helpButton = $state<HTMLButtonElement>()
  let helpPopover = $state<HTMLDivElement>()
  $effect(() => {
    if (!helpButton || !helpPopover) return
    return setupAnchorPopover({
      trigger: helpButton,
      popover: helpPopover,
      anchor: 'below-end',
      onOpenChange: () => {},
    })
  })
  const tabs = [
    { id: 'http', label: 'HTTP' },
    { id: 'grpc', label: 'gRPC' },
  ]
  const code = $derived(
    protocol === 'http'
      ? `export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:${httpPort ?? 4318}"\nexport OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"\nexport OTEL_SERVICE_NAME="my-service"`
      : 'export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"\nexport OTEL_EXPORTER_OTLP_PROTOCOL="grpc"\nexport OTEL_SERVICE_NAME="my-service"'
  )
  const example = $derived(
    `otel-cli exec \\\n  --endpoint http://localhost:${httpPort ?? 4318} \\\n  --protocol http/protobuf \\\n  --service my-service \\\n  --name "test trace" \\\n  -- sleep 1`
  )
  onMount(() => {
    let alive = true
    void telemetryAPI
      .getImportConfig()
      .then(config => {
        if (alive) httpPort = config.otlpHttpPort
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  })
</script>

<div class="getting-started">
  <div class="flex shrink-0 items-center gap-4 py-2">
    <img
      src={lulu}
      alt="Lulu the axolotl with her telescope"
      class="aspect-square w-2/5 max-w-40 shrink-0 object-contain"
    />
    <div class="flex min-w-0 flex-col gap-3">
      <h2
        class="welcome-title flex items-baseline gap-2 font-mono font-normal text-primary"
      >
        <span class="welcome-prompt shrink-0 select-none" aria-hidden="true"
          >&gt;</span
        >
        <span class="min-w-0 text-balance">otel-desktop-viewer</span>
      </h2>
      <p class="text-balance text-subtle">{tagline}</p>
    </div>
  </div>
  {#snippet guide()}
    <DetailBubble label="Getting started" contained headerless>
      <div class="guide-scroll">
        {@render importContent?.()}
        <section class="guide-section">
          <div class="flex flex-col gap-1">
            <h2 class="text-lg font-normal text-balance text-base-content">
              Connect an exporter
            </h2>
            <p class="text-pretty text-subtle">
              Run these commands, then start your app.
            </p>
          </div>
          <div class="terminal">
            <div class="flex items-center justify-between gap-2 px-3 py-2">
              <PaneHeader
                mode="tabs"
                bare
                {tabs}
                activeID={protocol}
                onSelect={id => (protocol = id)}
                ariaLabel="OTLP endpoint protocol"
                tabPanelID="home-otlp"
              />
              <div class="flex shrink-0 items-center gap-2">
                <button
                  type="button"
                  class="header-icon-button"
                  bind:this={helpButton}
                  popovertarget={helpID}
                  aria-label="Exporter connection help"
                  ><QuestionMarkIcon
                    width={16}
                    height={16}
                    aria-hidden="true"
                  /></button
                >
                <ValueCopyButton
                  text={code}
                  label="OTLP environment variables"
                  tooltip="Copy"
                />
              </div>
            </div>
            <div
              id={helpID}
              bind:this={helpPopover}
              popover="auto"
              role="note"
              class="anchor-popover anchor-popover--anchored connection-help p-3 text-sm"
            >
              <p>
                These variables configure OTLP over {protocol === 'grpc'
                  ? 'gRPC'
                  : 'HTTP with protobuf'}. Set them in the same terminal that
                starts your OpenTelemetry-instrumented app. If the viewer runs
                on another machine, replace <code>localhost</code> with its
                hostname or IP address.
                {#if protocol === 'grpc' || httpPort === null}
                  Port {protocol === 'grpc' ? 4317 : 4318} is the default {protocol ===
                  'grpc'
                    ? 'gRPC'
                    : 'HTTP'} port; use the viewer’s configured port if different.
                {/if}
              </p>
            </div>
            <div
              id="home-otlp"
              role="tabpanel"
              aria-labelledby={paneTabID('home-otlp', protocol)}
              tabindex="0"
              class="px-3 pb-3"
            >
              <ReadonlyCodePanel
                {code}
                bare
                ariaLabel="OTLP exporter environment variables"
              />
            </div>
          </div>
        </section>
        <section class="guide-section">
          <div class="flex flex-col gap-1">
            <h2 class="text-lg font-normal text-balance text-base-content">
              Send a test trace
            </h2>
            <p class="text-pretty text-subtle">
              Run this with <a
                class="text-secondary underline"
                href="https://github.com/equinix-labs/otel-cli#readme"
                target="_blank"
                rel="noreferrer">otel-cli</a
              > installed.
            </p>
          </div>
          <div class="terminal">
            <div class="flex items-center justify-between gap-2 px-3 py-2">
              <span class="text-subtle">Shell</span>
              <ValueCopyButton
                text={example}
                label="otel-cli example"
                tooltip="Copy"
              />
            </div>
            <div class="px-3 pb-3">
              <ReadonlyCodePanel
                bare
                ariaLabel="otel-cli example commands"
                code={example}
              />
            </div>
          </div>
        </section>
      </div>
    </DetailBubble>
  {/snippet}
  <DetailStack
    label="Getting started sections"
    storageKey="home-getting-started"
    sections={[{ id: 'guide', content: guide }, ...sections]}
  />
</div>

<style lang="postcss">
  @reference '../../app.css';
  .getting-started {
    @apply flex h-full min-h-0 min-w-0 flex-col p-3;
  }
  .welcome-title {
    font-size: 1.375rem;
    line-height: 1.25;
  }
  .welcome-prompt {
    color: var(--color-code-sora);
  }
  .guide-scroll {
    @apply flex min-h-0 flex-1 flex-col gap-8 overflow-y-auto p-3 text-sm;
  }
  .guide-section {
    @apply flex shrink-0 flex-col gap-3;
  }
  .terminal {
    @apply min-w-0 rounded-xl border-0 bg-base-100;
  }
  .connection-help {
    max-width: min(28rem, calc(100vw - 1rem));
  }
</style>
