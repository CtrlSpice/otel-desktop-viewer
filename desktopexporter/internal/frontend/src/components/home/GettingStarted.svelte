<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  import PaneHeader, { paneTabID } from '@/components/shared/PaneHeader.svelte'
  import ReadonlyCodePanel from '@/components/shared/ReadonlyCodePanel.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import Lulu from './Lulu.svelte'

  let { importContent }: { importContent?: Snippet } = $props()
  const taglines = [
    'We have OpenTelemetry at home',
    "I'm not changing the name, buddy.",
    'Now with “a lot more functionality”™',
    "I have splintered the ecosystem. Pray I don't splinter it any further.",
  ]
  const tagline = taglines[Math.floor(Math.random() * taglines.length)]
  let protocol = $state('http')
  let httpPort = $state<number | null>(null)
  let grpcPort = $state<number | null>(null)
  const panelID = $props.id()
  const tabs = [
    { id: 'http', label: 'HTTP' },
    { id: 'grpc', label: 'gRPC' },
  ]
  const code = $derived(
    `export OTEL_EXPORTER_OTLP_ENDPOINT=\\\n"http://localhost:${protocol === 'http' ? (httpPort ?? 4318) : (grpcPort ?? 4317)}"\nexport OTEL_EXPORTER_OTLP_PROTOCOL=\\\n"${protocol === 'http' ? 'http/protobuf' : 'grpc'}"\nexport OTEL_SERVICE_NAME="my-service"`
  )
  const example = $derived(
    `otel-cli exec \\\n  --endpoint http://localhost:${httpPort ?? 4318} \\\n  --protocol http/protobuf \\\n  --service my-service \\\n  --name "test trace" \\\n  -- sleep 1`
  )
  onMount(() => {
    const controller = new AbortController()
    void telemetryAPI
      .getImportConfig(controller.signal)
      .then(config => {
        if (!controller.signal.aborted) {
          httpPort = config.otlpHttpPort
          grpcPort = config.otlpGrpcPort ?? null
        }
      })
      .catch(() => {
        // The guide labels the default port when configuration is unavailable.
      })
    return () => controller.abort()
  })
</script>

<header class="guide-welcome">
  <div class="guide-lulu"><Lulu /></div>
  <div class="guide-wordmark">
    <h1 class="guide-name">OTel Desktop Viewer</h1>
    <p class="guide-subtitle">{tagline}</p>
  </div>
</header>
<div class="guide-scroll">
  {#if importContent}
    <div class="guide-import">{@render importContent()}</div>
  {/if}
  <section class="flex flex-col gap-2">
    <h2 class="text-lg font-normal">Connect an exporter</h2>
    <p class="text-subtle">
      Set these variables in the terminal where you start your instrumented app.
    </p>
    <div
      class="overflow-hidden rounded-xl border border-(--color-highlight-high) bg-base-100"
    >
      <div
        class="flex items-center justify-between gap-2 bg-base-200 px-3 py-2"
      >
        <PaneHeader
          mode="tabs"
          bare
          {tabs}
          activeID={protocol}
          onSelect={id => (protocol = id)}
          ariaLabel="OTLP endpoint protocol"
          tabPanelID={panelID}
        />
        <ValueCopyButton
          text={code}
          label="OTLP environment variables"
          tooltip="Copy"
        />
      </div>
      <div
        id={panelID}
        role="tabpanel"
        aria-labelledby={paneTabID(panelID, protocol)}
        tabindex="0"
        class="p-3"
      >
        <ReadonlyCodePanel
          {code}
          bare
          wrap={false}
          ariaLabel="OTLP exporter environment variables"
        />
      </div>
    </div>
    {#if (protocol === 'grpc' ? grpcPort : httpPort) === null}<p
        class="text-subtle"
      >
        Port {protocol === 'grpc' ? 4317 : 4318} is the default. Use the viewer’s
        configured port if different.
      </p>{/if}
  </section>
  <section class="flex flex-col gap-2">
    <h2 class="text-lg font-normal">Send a test trace</h2>
    <p class="text-subtle">
      Run this with <a
        class="text-secondary underline"
        href="https://github.com/equinix-labs/otel-cli#readme"
        target="_blank"
        rel="noreferrer">otel-cli</a
      > installed.
    </p>
    <div
      class="overflow-hidden rounded-xl border border-(--color-highlight-high) bg-base-100"
    >
      <div
        class="flex items-center justify-between gap-2 bg-base-200 px-3 py-2"
      >
        <span class="text-subtle">Shell</span>
        <ValueCopyButton
          text={example}
          label="otel-cli example"
          tooltip="Copy"
        />
      </div>
      <div class="p-3">
        <ReadonlyCodePanel
          code={example}
          bare
          wrap={false}
          ariaLabel="otel-cli example commands"
        />
      </div>
    </div>
  </section>
</div>

<style lang="postcss">
  @reference '../../app.css';
  .guide-scroll {
    @apply flex min-h-0 flex-1 flex-col gap-8 overflow-y-auto py-3;
    padding-block-start: 2rem;
  }
  .guide-import {
    @apply flex h-24 shrink-0;
  }
  .guide-lulu {
    width: 100%;
    min-width: 0;
    aspect-ratio: 32 / 27;
  }
  .guide-welcome {
    @apply grid shrink-0 items-center gap-x-3;
    grid-template-columns: minmax(0, 48px) minmax(min-content, 1fr);
    min-height: var(--pane-header-single-h);
    padding-inline: var(--pane-inline-inset);
  }
  .guide-wordmark {
    @apply flex min-w-0 flex-col;
    gap: 0.125rem;
  }
  .guide-name {
    @apply whitespace-nowrap font-normal text-base-content;
    font-size: 1.5rem;
    line-height: 1.15;
    letter-spacing: -0.02em;
  }
  .guide-subtitle {
    @apply text-balance text-sm text-subtle;
  }
</style>
