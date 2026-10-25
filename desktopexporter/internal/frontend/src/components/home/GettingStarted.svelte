<script lang="ts">
  import { onMount } from 'svelte'
  import PaneHeader, { paneTabID } from '@/components/shared/PaneHeader.svelte'
  import ReadonlyCodePanel from '@/components/shared/ReadonlyCodePanel.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'

  let protocol = $state('http')
  let httpPort = $state<number | null>(null)
  const panelID = $props.id()
  const tabs = [
    { id: 'http', label: 'HTTP' },
    { id: 'grpc', label: 'gRPC' },
  ]
  const code = $derived(
    `export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:${protocol === 'http' ? (httpPort ?? 4318) : 4317}"\nexport OTEL_EXPORTER_OTLP_PROTOCOL="${protocol === 'http' ? 'http/protobuf' : 'grpc'}"\nexport OTEL_SERVICE_NAME="my-service"`
  )
  const example = $derived(
    `otel-cli exec \\\n  --endpoint http://localhost:${httpPort ?? 4318} \\\n  --protocol http/protobuf \\\n  --service my-service \\\n  --name "test trace" \\\n  -- sleep 1`
  )
  onMount(() => {
    const controller = new AbortController()
    void telemetryAPI
      .getImportConfig(controller.signal)
      .then(config => {
        if (!controller.signal.aborted) httpPort = config.otlpHttpPort
      })
      .catch(() => {
        // The guide labels the default port when configuration is unavailable.
      })
    return () => controller.abort()
  })
</script>

<div class="guide-scroll">
  <section class="flex flex-col gap-3">
    <h2 class="text-lg font-normal">Connect an exporter</h2>
    <p class="text-subtle">
      Set these variables in the terminal that starts your
      OpenTelemetry-instrumented app.
    </p>
    <div
      class="overflow-hidden rounded-xl border border-(--color-highlight-high) bg-base-100"
    >
      <div
        class="flex items-center justify-between gap-2 bg-base-300 px-3 py-2"
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
          ariaLabel="OTLP exporter environment variables"
        />
      </div>
    </div>
    {#if protocol === 'grpc' || httpPort === null}<p class="text-subtle">
        Port {protocol === 'grpc' ? 4317 : 4318} is the default. Use the viewer’s
        configured port if different.
      </p>{/if}
  </section>
  <section class="flex flex-col gap-3">
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
        class="flex items-center justify-between gap-2 bg-base-300 px-3 py-2"
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
          ariaLabel="otel-cli example commands"
        />
      </div>
    </div>
  </section>
</div>

<style lang="postcss">
  @reference '../../app.css';
  .guide-scroll {
    @apply flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto p-3;
  }
</style>
