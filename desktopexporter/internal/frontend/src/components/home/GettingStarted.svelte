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
      ? `export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:${httpPort ?? 4318}"\nexport OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"`
      : 'export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"\nexport OTEL_EXPORTER_OTLP_PROTOCOL="grpc"'
  )
  const example = $derived(
    `export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:${httpPort ?? 4318}\notel-cli exec --service my-service --name "curl google" curl https://google.com`
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
  <div class="flex shrink-0 items-center gap-3 px-3 py-2">
    <img
      src={lulu}
      alt="Lulu the axolotl with her telescope"
      class="size-24 shrink-0 object-contain"
    />
    <div class="min-w-0">
      <h2 class="text-lg font-normal text-base-content">
        Welcome to otel-desktop-viewer
      </h2>
      <p class="text-rp-subtle">A cozy place for your telemetry.</p>
    </div>
  </div>
  <div class="guide-scroll">
    {@render importContent?.()}
    <section class="guide-section">
      <h2 class="text-lg font-normal text-base-content">Connect an exporter</h2>
      <p class="text-rp-subtle">
        Send telemetry from your SDK, Collector, or agent.
      </p>
      <div class="flex items-center justify-between gap-2">
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
      >
        <ReadonlyCodePanel
          {code}
          bare
          ariaLabel="OTLP exporter environment variables"
        />
      </div>
      <p class="text-rp-subtle">
        {protocol === 'grpc'
          ? 'Default gRPC port. Use your configured port if different.'
          : httpPort === null
            ? 'Default HTTP port. Use your configured port if different.'
            : 'Configured HTTP port.'} Use the viewer host for a remote exporter.
      </p>
    </section>
    <section class="guide-section">
      <div class="flex items-center justify-between gap-2">
        <h2 class="text-lg font-normal text-base-content">Try an example</h2>
        <ValueCopyButton
          text={example}
          label="otel-cli example"
          tooltip="Copy"
        />
      </div>
      <ReadonlyCodePanel
        bare
        ariaLabel="otel-cli example commands"
        code={example}
      />
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
    @apply flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto p-3 text-sm;
  }
  .guide-section {
    @apply flex shrink-0 flex-col gap-2;
  }
</style>
