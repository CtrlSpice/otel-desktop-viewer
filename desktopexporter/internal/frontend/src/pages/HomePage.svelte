<script lang="ts">
  import { onMount } from "svelte"
  import { HugeiconsIcon } from "@hugeicons/svelte"
  import {
    BarChartHorizontalIcon,
    Chart03Icon,
    FirePitIcon,
  } from "@hugeicons/core-free-icons"
  import CodeBlock from "@/components/CodeBlock.svelte"
  import { telemetryAPI } from "@/services/telemetry-service"
  import type { Stats } from "@/types/api-types"
  import luluImage from "@/assets/images/lulu.png"
  import axolotlImage from "@/assets/images/axolotl.svg"

  const POLL_INTERVAL_MS = 5000;

  let stats = $state<Stats | null>(null);

  async function fetchStats() {
    try {
      stats = await telemetryAPI.getStats();
    } catch (e) {
      console.error('Failed to fetch stats:', e);
    }
  }

  function formatRelativeTime(timestampNs: bigint | null | undefined): string {
    if (timestampNs == null) return '-';
    const ms = Number(timestampNs / 1_000_000n);
    const seconds = Math.floor((Date.now() - ms) / 1000);
    if (seconds < 5) return 'just now';
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
  }

  onMount(() => {
    fetchStats();
    const pollTimer = setInterval(fetchStats, POLL_INTERVAL_MS);
    return () => clearInterval(pollTimer);
  });
</script>

<!-- HomePage.svelte - The welcome/landing page -->
<div class="min-w-0 py-6">
  <!-- Header Section -->
  <header class="mb-10">
    <div
      class="flex flex-col items-center gap-8 text-center min-[1100px]:flex-row min-[1100px]:items-center min-[1100px]:gap-10 min-[1100px]:text-left"
    >
      <img
        src={luluImage}
        alt="A pink axolotl is striking a heroic pose while gazing at a field of stars through a telescope. Her name is Lulu Axol'Otel the First, valiant adventurer and observability queen."
        class="h-48 w-48 shrink-0 object-contain drop-shadow-sm min-[1100px]:h-64 min-[1100px]:w-64"
      />
      <div class="min-w-0 py-2">
        <p
          class="mb-2 text-xs font-semibold uppercase tracking-[0.12em] text-primary/80"
        >
          Local-first telemetry
        </p>
        <h1
          class="mb-3 text-3xl font-semibold leading-tight tracking-tight text-base-content min-[1100px]:text-4xl"
        >
          OpenTelemetry Desktop Viewer
        </h1>
        <p class="max-w-xl text-base leading-relaxed text-base-content/65">
          Collect, visualize, and query your OpenTelemetry data locally — without
          shipping it to the cloud.
        </p>
      </div>
    </div>
  </header>

  <!-- Summary Sections -->
  <div class="summary-grid">
    <!-- Traces Summary -->
    <div class="summary-card">
      <div class="summary-header">
        <div class="summary-icon">
          <span class="text-secondary" aria-hidden="true">
            <HugeiconsIcon icon={BarChartHorizontalIcon} size={18} color="currentColor" />
          </span>
        </div>
        <h3 class="summary-title">Traces</h3>
      </div>
      <div class="summary-stats">
        <div class="summary-stat">
          <span class="summary-label">Traces</span>
          <span class="summary-value">{stats?.traces.traceCount ?? 0}</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Spans</span>
          <span class="summary-value">{stats?.traces.spanCount ?? 0}</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Services</span>
          <span class="summary-value">{stats?.traces.serviceCount ?? 0}</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Errors</span>
          <span class="summary-value">{stats?.traces.errorCount ?? 0}</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Last Received</span>
          <span class="summary-value">{formatRelativeTime(stats?.traces.lastReceived ?? null)}</span>
        </div>
      </div>
      <div class="summary-footer">
        <a href="/traces" class="summary-link">View all traces →</a>
      </div>
    </div>

    <!-- Metrics Summary -->
    <div class="summary-card">
      <div class="summary-header">
        <div class="summary-icon">
          <span class="text-secondary" aria-hidden="true">
            <HugeiconsIcon icon={Chart03Icon} size={18} color="currentColor" />
          </span>
        </div>
        <h3 class="summary-title">Metrics</h3>
      </div>
      <div class="summary-stats">
        <div class="summary-stat">
          <span class="summary-label">Metrics</span>
          <span class="summary-value">{stats?.metrics.metricCount ?? 0}</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Data Points</span>
          <span class="summary-value">{stats?.metrics.dataPointCount ?? 0}</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Last Received</span>
          <span class="summary-value">{formatRelativeTime(stats?.metrics.lastReceived ?? null)}</span>
        </div>
      </div>
      <div class="summary-footer">
        <a href="/metrics" class="summary-link">View all metrics →</a>
      </div>
    </div>

    <!-- Logs Summary -->
    <div class="summary-card">
      <div class="summary-header">
        <div class="summary-icon">
          <span class="text-secondary" aria-hidden="true">
            <HugeiconsIcon icon={FirePitIcon} size={18} color="currentColor" />
          </span>
        </div>
        <h3 class="summary-title">Logs</h3>
      </div>
      <div class="summary-stats">
        <div class="summary-stat">
          <span class="summary-label">Logs</span>
          <span class="summary-value">{stats?.logs.logCount ?? 0}</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Errors</span>
          <span class="summary-value">{stats?.logs.errorCount ?? 0}</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Last Received</span>
          <span class="summary-value">{formatRelativeTime(stats?.logs.lastReceived ?? null)}</span>
        </div>
      </div>
      <div class="summary-footer">
        <a href="/logs" class="summary-link">View all logs →</a>
      </div>
    </div>
  </div>

  <!-- Configuration Section -->
  <div class="section">
    <h2 class="section-title">Configure your OTLP exporter</h2>
    
    <!-- HTTP Configuration -->
    <div class="config-group">
      <h3 class="config-subtitle">HTTP Endpoint</h3>
      <CodeBlock code={`$ export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
$ export OTEL_TRACES_EXPORTER="otlp"
$ export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"`} />
    </div>

    <!-- GRPC Configuration -->
    <div class="config-group">
      <h3 class="config-subtitle">GRPC Endpoint</h3>
      <CodeBlock code={`$ export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"
$ export OTEL_TRACES_EXPORTER="otlp"
$ export OTEL_EXPORTER_OTLP_PROTOCOL="grpc"`} />
    </div>
  </div>

  <!-- Example Section -->
  <div class="section">
    <h2 class="section-title">Example with otel-cli</h2>
    <p class="section-description">
      If you have 
      <a href="https://github.com/equinix-labs/otel-cli" 
         class="external-link" 
         target="_blank" 
         rel="noopener noreferrer">
        otel-cli
      </a> 
      installed, you can send example data:
    </p>
    <CodeBlock code={`# start the desktop viewer
$ otel-desktop-viewer

# configure otel-cli
$ export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318

# generate spans!
$ otel-cli exec --service my-service --name "curl google" curl https://google.com`} />
  </div>

  <!-- Footer -->
  <div class="footer">
    <p class="footer-text">
      Made with 
      <img
        src={axolotlImage}
        alt="axolotl emoji"
        class="footer-icon"
      /> 
      by 
      <a href="https://github.com/CtrlSpice" 
         class="footer-link" 
         target="_blank" 
         rel="noopener noreferrer">
        Mila Ardath
      </a>
      , with Artwork by 
      <a href="https://cbatesonart.artstation.com/" 
         class="footer-link" 
         target="_blank" 
         rel="noopener noreferrer">
        Chelsey Bateson
      </a>
    </p>
  </div>
</div>

<style lang="postcss">
  /* Summary Cards */
  .summary-grid {
    @apply mb-10 grid gap-5;
    grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
  }
  
  .summary-card {
    @apply flex flex-col rounded-2xl border border-base-300/60 bg-base-200/40 p-5 shadow-surface-sm backdrop-blur-sm transition-[border-color,box-shadow] duration-200;
  }

  .summary-card:hover {
    @apply border-base-300 shadow-surface;
  }
  
  .summary-header {
    @apply mb-5 flex items-center gap-3;
  }
  
  .summary-icon {
    @apply flex h-10 w-10 items-center justify-center rounded-xl bg-secondary/15 text-secondary ring-1 ring-secondary/20;
  }
  
  .summary-title {
    @apply text-lg font-semibold tracking-tight;
  }
  
  .summary-stats {
    @apply flex-grow space-y-2.5;
  }
  
  .summary-stat {
    @apply flex items-baseline justify-between gap-3 text-sm;
  }
  
  .summary-label {
    @apply text-base-content/60;
  }
  
  .summary-value {
    @apply tabular-nums font-medium tracking-tight text-base-content;
    transition: opacity 150ms ease;
  }
  
  .summary-footer {
    @apply mt-auto border-t border-base-300/70 pt-4;
  }
  
  .summary-link {
    @apply text-sm font-medium text-primary underline-offset-4 transition-colors hover:text-primary/85 hover:underline;
  }

  
  /* Sections */
  .section {
    @apply mb-8;
  }
  
  .section-title {
    @apply mb-5 text-2xl font-semibold tracking-tight;
  }
  
  .section-description {
    @apply mb-6 max-w-2xl leading-relaxed text-base-content/65;
  }
  
  /* Configuration */
  .config-group {
    @apply space-y-4 mb-8;
  }
  
  .config-group:last-child {
    @apply mb-0;
  }
  
  .config-subtitle {
    @apply text-lg font-medium;
  }
  
  /* Links */
  .external-link {
    @apply link link-primary;
  }
  
  /* Footer */
  .footer {
    @apply border-t border-base-300 pt-8 flex justify-center;
  }
  
  .footer-text {
    @apply text-sm text-base-content/60 text-center block;
  }
  
  .footer-icon {
    @apply inline w-5 h-5 mx-1;
  }
  
  .footer-link {
    @apply link link-primary;
  }
  
</style>