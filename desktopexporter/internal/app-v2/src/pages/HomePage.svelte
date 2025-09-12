<script lang="ts">
  import { telemetryAPI } from "../services/telemetry-service"
  import { HugeiconsIcon } from "@hugeicons/svelte"
  import {
    BarChartHorizontalIcon,
    Chart03Icon,
    File01Icon,
    Add01Icon,
    Delete01Icon,
  } from "@hugeicons/core-free-icons"
  import { onMount } from "svelte"

  let sampleDataExists = false
  let loading = false

  // Check if sample data exists on page load
  onMount(async () => {
    try {
      const response = await telemetryAPI.checkSampleDataExists()
      sampleDataExists = response.exists
    } catch (error) {
      console.error("Error checking sample data status:", error)
    }
  })

  async function loadSampleData() {
    loading = true
    try {
      await telemetryAPI.loadSampleData()
      sampleDataExists = true
    } catch (error) {
      console.error("Error loading sample data:", error)
    } finally {
      loading = false
    }
  }

  async function clearSampleData() {
    loading = true
    try {
      await telemetryAPI.clearSampleData()
      sampleDataExists = false
    } catch (error) {
      console.error("Error clearing sample data:", error)
    } finally {
      loading = false
    }
  }
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
          <span class="summary-label">Total Traces</span>
          <span class="summary-value">0</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Services</span>
          <span class="summary-value">0</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Avg Duration</span>
          <span class="summary-value">-</span>
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
          <span class="summary-label">Metric Types</span>
          <span class="summary-value">0</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Data Points</span>
          <span class="summary-value">0</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Last Updated</span>
          <span class="summary-value">-</span>
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
          <HugeiconsIcon icon={File01Icon} size={16} color="hsl(var(--s))" />
        </div>
        <h3 class="summary-title">Logs</h3>
      </div>
      <div class="summary-stats">
        <div class="summary-stat">
          <span class="summary-label">Total Logs</span>
          <span class="summary-value">0</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Error Level</span>
          <span class="summary-value">0</span>
        </div>
        <div class="summary-stat">
          <span class="summary-label">Last Log</span>
          <span class="summary-value">-</span>
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
      <div class="code-block">
        <pre><code><span class="prompt">$</span> export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
<span class="prompt">$</span> export OTEL_TRACES_EXPORTER="otlp"
<span class="prompt">$</span> export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"</code></pre>
      </div>
    </div>

    <!-- GRPC Configuration -->
    <div class="config-group">
      <h3 class="config-subtitle">GRPC Endpoint</h3>
      <div class="code-block">
        <pre><code><span class="prompt">$</span> export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"
<span class="prompt">$</span> export OTEL_TRACES_EXPORTER="otlp"
<span class="prompt">$</span> export OTEL_EXPORTER_OTLP_PROTOCOL="grpc"</code></pre>
      </div>
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
    <div class="code-block">
      <pre><code><span class="comment"># start the desktop viewer</span>
<span class="prompt">$</span> otel-desktop-viewer

<span class="comment"># configure otel-cli</span>
<span class="prompt">$</span> export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318

<span class="comment"># generate spans!</span>
<span class="prompt">$</span> otel-cli exec --service my-service --name "curl google" curl https://google.com</code></pre>
    </div>
  </div>

  <!-- Sample Data Section -->
  <div class="section">
    <h2 class="section-title">Try with sample data</h2>
    
    <p class="section-description {sampleDataExists ? 'success' : ''}">
      {#if sampleDataExists}
        Great! Sample telemetry data is available. You can now explore traces, logs, and metrics in the application.
      {:else}
        Want to see it in action? Load some sample telemetry data to explore the application immediately.
      {/if}
    </p>
    
    <button 
      class="sample-data-btn {sampleDataExists ? 'delete' : 'load'}" 
      onclick={sampleDataExists ? clearSampleData : loadSampleData}
      disabled={loading}
    >
      {#if loading}
        <span class="loading loading-spinner loading-sm"></span>
        {sampleDataExists ? 'Clearing...' : 'Loading...'}
      {:else}
        <HugeiconsIcon 
          icon={sampleDataExists ? Delete01Icon : Add01Icon} 
          size={16} 
        />
        {sampleDataExists ? 'Clear sample data' : 'Load sample data'}
      {/if}
    </button>
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

<style>
  /* Summary Cards */
  .summary-grid {
    @apply grid md:grid-cols-3 gap-6 mb-8;
  }
  
  .summary-card {
    @apply bg-base-200 border border-base-300 rounded-lg p-6;
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
    @apply space-y-2;
  }
  
  .summary-stat {
    @apply flex justify-between text-sm;
  }
  
  .summary-label {
    @apply text-base-content/70;
  }
  
  .summary-value {
    @apply font-medium;
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
  
  .section-description.success {
    @apply text-success;
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
  
  /* Sample Data Button */
  .sample-data-btn {
    @apply btn btn-outline min-w-[180px] gap-2;
  }
  
  .sample-data-btn.load {
    @apply hover:bg-primary hover:text-primary-content hover:border-primary;
  }
  
  .sample-data-btn.delete {
    @apply hover:bg-error hover:text-error-content hover:border-error;
  }
</style>