<script lang="ts">
  import type { Snippet } from 'svelte'
  import { onMount } from 'svelte'
  import PaneHeader, {
    paneTabID,
    type PaneTab,
  } from '@/components/shared/PaneHeader.svelte'
  import ReadonlyCodePanel from '@/components/shared/ReadonlyCodePanel.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import luluImage from '@/assets/images/lulu.webp'

  let { importContent }: { importContent?: Snippet } = $props()
  let protocol = $state('http')
  let httpPort = $state<number | null>(null)
  const tabs: PaneTab[] = [
    { id: 'http', label: 'HTTP' },
    { id: 'grpc', label: 'gRPC' },
  ]
  const code = $derived(
    protocol === 'http'
      ? `export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:${httpPort ?? 4318}"\nexport OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"`
      : 'export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"\nexport OTEL_EXPORTER_OTLP_PROTOCOL="grpc"'
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
  <PaneHeader
    mode="title"
    pill
    title="Getting started"
    ariaLabel="Getting started"
  />
  <div class="guide-scroll">
    {@render importContent?.()}
    <section class="guide-section">
      <h2 class="text-lg font-normal text-base-content">Connect an exporter</h2>
      <p class="text-rp-subtle">
        Send telemetry from your SDK, Collector, or agent.
      </p>
      <PaneHeader
        mode="tabs"
        pill
        {tabs}
        activeID={protocol}
        onSelect={id => (protocol = id)}
        ariaLabel="OTLP endpoint protocol"
        tabPanelID="home-otlp"
      />
      <div
        id="home-otlp"
        role="tabpanel"
        aria-labelledby={paneTabID('home-otlp', protocol)}
        tabindex="0"
      >
        <ReadonlyCodePanel
          {code}
          ariaLabel="OTLP exporter environment variables"
          copyLabel="Copy OTLP environment variables"
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
      <h2 class="text-lg font-normal text-base-content">Try an example</h2>
      <FieldGroup detail label="Send a trace with otel-cli" open={false}>
        <ReadonlyCodePanel
          ariaLabel="otel-cli example commands"
          code={`export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:${httpPort ?? 4318}\notel-cli exec --service my-service --name "curl google" curl https://google.com`}
        />
      </FieldGroup>
      <a
        class="link text-secondary"
        href="https://opentelemetry.io/docs/languages/"
        target="_blank"
        rel="noopener noreferrer">SDK setup</a
      >
    </section>
    <footer class="guide-footer">
      <img
        src={luluImage}
        alt="Lulu the pink axolotl looking through her telescope."
        class="size-24 object-contain"
      />
      <p class="text-rp-subtle">
        Artwork by <a
          class="link text-secondary"
          href="https://cbatesonart.artstation.com/"
          target="_blank"
          rel="noopener noreferrer">Chelsey Bateson</a
        >
      </p>
    </footer>
  </div>
</div>

<style lang="postcss">
  @reference '../../app.css';
  .getting-started {
    @apply flex h-full min-h-0 min-w-0 flex-col pt-3;
  }
  .guide-scroll {
    @apply min-h-0 flex-1 overflow-y-auto p-3 text-sm;
  }
  .guide-section {
    @apply flex flex-col gap-2 py-3;
  }
  .guide-footer {
    @apply flex flex-col items-center gap-2 py-4 text-center;
  }
</style>
