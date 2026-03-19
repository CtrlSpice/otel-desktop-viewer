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

  const POLL_INTERVAL_MS = 5000;

  let stats = $state<Stats | null>(null);

  async function fetchStats() {
    try {
      stats = await telemetryAPI.getStats();
    } catch (e) {
      console.error('Failed to fetch stats:', e);
    }
  }

  function formatRelativeTime(timestampNs: number | null): string {
    if (timestampNs == null) return '-';
    const ms = timestampNs / 1_000_000;
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
<div class="max-w-6xl mx-auto px-6 py-12">
  <!-- Header Section -->
  <div>
    <div class="flex items-center gap-6">
      <img
        src="/src/assets/images/lulu.png"
        alt="A pink axolotl is striking a heroic pose while gazing at a field of stars through a telescope. Her name is Lulu Axol'Otel the First, valiant adventurer and observability queen."
        class="w-64 h-64 object-contain"
      />
      <div>
        <h1 class="text-2xl font-bold">OpenTelemetry Desktop Viewer</h1>
        <p class="text-base-content/70">
          Collect, visualize, and query your OpenTelemetry data locally.
        </p>
      </div>
    </div>
  </div>

  <!-- Summary Sections -->
  <div class="summary-grid">
    <!-- Traces Summary -->
    <div class="summary-card">
      <div class="summary-header">
        <div class="summary-icon">
          <HugeiconsIcon icon={BarChartHorizontalIcon} size={16} color="hsl(var(--s))" />
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
          <HugeiconsIcon icon={Chart03Icon} size={16} color="hsl(var(--s))" />
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
          <HugeiconsIcon icon={FirePitIcon} size={16} color="hsl(var(--s))" />
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
        src="/src/assets/images/axolotl.svg"
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
    @apply grid md:grid-cols-3 gap-6 mb-8;
  }
  
  .summary-card {
    @apply bg-base-200 border border-base-300 rounded-lg p-4 flex flex-col;
  }
  
  .summary-header {
    @apply flex items-center gap-3 mb-4;
  }
  
  .summary-icon {
    @apply w-8 h-8 bg-secondary/20 rounded-lg flex items-center justify-center;
  }
  
  .summary-title {
    @apply text-lg font-semibold;
  }
  
  .summary-stats {
    @apply space-y-2 flex-grow;
  }
  
  .summary-stat {
    @apply flex justify-between text-sm;
  }
  
  .summary-label {
    @apply text-base-content/70;
  }
  
  .summary-value {
    @apply font-medium;
    transition: opacity 150ms ease;
  }
  
  .summary-footer {
    @apply mt-4 pt-4 border-t border-base-300;
  }
  
  .summary-link {
    @apply text-primary text-sm hover:underline;
  }

  
  /* Sections */
  .section {
    @apply mb-8;
  }
  
  .section-title {
    @apply text-2xl font-semibold mb-6;
  }
  
  .section-description {
    @apply text-base-content/70 mb-6;
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