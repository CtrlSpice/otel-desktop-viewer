<script lang="ts">
  import type { Snippet } from 'svelte'
  import { onMount } from 'svelte'
  import PaneHeader, { paneTabID } from '@/components/shared/PaneHeader.svelte'
  import ReadonlyCodePanel from '@/components/shared/ReadonlyCodePanel.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import lulu from '@/assets/images/lulu.webp'

  let { importContent, issues }: { importContent?: Snippet; issues?: Snippet } =
    $props()
  let protocol = $state('http')
  let httpPort = $state<number | null>(null)
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
  <div class="flex shrink-0 items-center gap-4 px-3 py-6">
    <img
      src={lulu}
      alt="Lulu the axolotl with her telescope"
      class="size-48 shrink-0 object-contain"
    />
    <div class="flex min-w-0 flex-col gap-2">
      <h2 class="text-lg font-normal text-balance text-base-content">
        Welcome to otel-desktop-viewer
      </h2>
      <p class="text-pretty text-rp-subtle">A cozy place for your telemetry.</p>
    </div>
  </div>
  <div class="guide-scroll">
    {@render importContent?.()}
    <section class="guide-section">
      <div class="flex flex-col gap-1">
        <h2 class="text-lg font-normal text-balance text-base-content">
          Connect an exporter
        </h2>
        <p class="text-pretty text-rp-subtle">
          Run these commands in your terminal, then start your app with
          OpenTelemetry enabled. If your app runs on another machine, replace <code
            >localhost</code
          >
          with this viewer’s hostname or IP address.
          {#if protocol === 'grpc' || httpPort === null}
            Replace port {protocol === 'grpc' ? 4317 : 4318} if your viewer uses a
            different port.
          {/if}
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
          <ValueCopyButton
            text={code}
            label="OTLP environment variables"
            tooltip="Copy"
          />
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
        <p class="text-pretty text-rp-subtle">
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
          <span class="text-rp-subtle">Shell</span>
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
    <div class="mt-auto flex shrink-0 flex-col gap-3">
      {@render issues?.()}
    </div>
  </div>
</div>

<style lang="postcss">
  @reference '../../app.css';
  .getting-started {
    @apply flex h-full min-h-0 min-w-0 flex-col pt-3;
  }
  .guide-scroll {
    @apply flex min-h-0 flex-1 flex-col gap-8 overflow-y-auto p-3 text-sm;
  }
  .guide-section {
    @apply flex shrink-0 flex-col gap-3;
  }
  .terminal {
    @apply min-w-0 rounded-xl border border-base-300 bg-base-100;
  }
</style>
