<script lang="ts">
  import { telemetryAPI } from "../services/telemetry-service"
  import { HugeiconsIcon } from "@hugeicons/svelte"
  import {
    BarChartHorizontalIcon,
    Chart03Icon,
    File01Icon,
  } from "@hugeicons/core-free-icons"

  let loadingSampleData = false
  let sampleDataError: string | null = null
  let sampleDataSuccess = false

  async function loadSampleData() {
    try {
      loadingSampleData = true
      sampleDataError = null
      sampleDataSuccess = false
      
      await telemetryAPI.loadSampleData()
      sampleDataSuccess = true
    } catch (error) {
      sampleDataError = error instanceof Error ? error.message : "Failed to load sample data"
      console.error("Error loading sample data:", error)
    } finally {
      loadingSampleData = false
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
  <div class="grid md:grid-cols-3 gap-6 mb-8">
    <!-- Traces Summary -->
    <div class="bg-base-200 border border-base-300 rounded-lg p-6">
      <div class="flex items-center gap-3 mb-4">
        <div class="w-8 h-8 bg-secondary/20 rounded-lg flex items-center justify-center">
          <HugeiconsIcon icon={BarChartHorizontalIcon} size={16} color="hsl(var(--s))" />
        </div>
        <h3 class="text-lg font-semibold">Traces</h3>
      </div>
      <div class="space-y-2">
        <div class="flex justify-between text-sm">
          <span class="text-base-content/70">Total Traces</span>
          <span class="font-medium">0</span>
        </div>
        <div class="flex justify-between text-sm">
          <span class="text-base-content/70">Services</span>
          <span class="font-medium">0</span>
        </div>
        <div class="flex justify-between text-sm">
          <span class="text-base-content/70">Avg Duration</span>
          <span class="font-medium">-</span>
        </div>
      </div>
      <div class="mt-4 pt-4 border-t border-base-300">
        <a href="/traces" class="text-primary text-sm hover:underline">View all traces →</a>
      </div>
    </div>

    <!-- Metrics Summary -->
    <div class="bg-base-200 border border-base-300 rounded-lg p-6">
      <div class="flex items-center gap-3 mb-4">
        <div class="w-8 h-8 bg-secondary/20 rounded-lg flex items-center justify-center">
          <HugeiconsIcon icon={Chart03Icon} size={16} color="hsl(var(--s))" />
        </div>
        <h3 class="text-lg font-semibold">Metrics</h3>
      </div>
      <div class="space-y-2">
        <div class="flex justify-between text-sm">
          <span class="text-base-content/70">Metric Types</span>
          <span class="font-medium">0</span>
        </div>
        <div class="flex justify-between text-sm">
          <span class="text-base-content/70">Data Points</span>
          <span class="font-medium">0</span>
        </div>
        <div class="flex justify-between text-sm">
          <span class="text-base-content/70">Last Updated</span>
          <span class="font-medium">-</span>
        </div>
      </div>
      <div class="mt-4 pt-4 border-t border-base-300">
        <a href="/metrics" class="text-primary text-sm hover:underline">View all metrics →</a>
      </div>
    </div>

    <!-- Logs Summary -->
    <div class="bg-base-200 border border-base-300 rounded-lg p-6">
      <div class="flex items-center gap-3 mb-4">
        <div class="w-8 h-8 bg-secondary/20 rounded-lg flex items-center justify-center">
          <HugeiconsIcon icon={File01Icon} size={16} color="hsl(var(--s))" />
        </div>
        <h3 class="text-lg font-semibold">Logs</h3>
      </div>
      <div class="space-y-2">
        <div class="flex justify-between text-sm">
          <span class="text-base-content/70">Total Logs</span>
          <span class="font-medium">0</span>
        </div>
        <div class="flex justify-between text-sm">
          <span class="text-base-content/70">Error Level</span>
          <span class="font-medium">0</span>
        </div>
        <div class="flex justify-between text-sm">
          <span class="text-base-content/70">Last Log</span>
          <span class="font-medium">-</span>
        </div>
      </div>
      <div class="mt-4 pt-4 border-t border-base-300">
        <a href="/logs" class="text-primary text-sm hover:underline">View all logs →</a>
      </div>
    </div>
  </div>


  <!-- Configuration Section -->
  <div class="mb-8">
    <h2 class="text-2xl font-semibold mb-6">Configure your OTLP exporter</h2>
    
    <!-- HTTP Configuration -->
    <div class="space-y-4 mb-8">
      <h3 class="text-lg font-medium">HTTP Endpoint</h3>
      <div class="code-block">
        <pre><code><span class="prompt">$</span> export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
<span class="prompt">$</span> export OTEL_TRACES_EXPORTER="otlp"
<span class="prompt">$</span> export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"</code></pre>
      </div>
    </div>

    <!-- GRPC Configuration -->
    <div class="space-y-4">
      <h3 class="text-lg font-medium">GRPC Endpoint</h3>
      <div class="code-block">
        <pre><code><span class="prompt">$</span> export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"
<span class="prompt">$</span> export OTEL_TRACES_EXPORTER="otlp"
<span class="prompt">$</span> export OTEL_EXPORTER_OTLP_PROTOCOL="grpc"</code></pre>
      </div>
    </div>
    
  </div>

  <!-- Example Section -->
  <div class="mb-8">
    <h2 class="text-2xl font-semibold mb-6">Example with otel-cli</h2>
    <p class="text-base-content/70 mb-4">
      If you have 
      <a href="https://github.com/equinix-labs/otel-cli" 
         class="link link-primary" 
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
  <div class="mb-8">
    <h2 class="text-2xl font-semibold mb-6">Try with sample data</h2>
    <p class="text-base-content/70 mb-6">
      Want to see it in action? Load some sample telemetry data to explore the application immediately.
    </p>
    
    <div class="flex items-center gap-4">
      <button 
        class="btn btn-neutral hover:btn-primary" 
        onclick={loadSampleData}
        disabled={loadingSampleData}
      >
        {#if loadingSampleData}
          <span class="loading loading-spinner loading-sm"></span>
          Loading sample data...
        {:else}
          <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
          </svg>
          Load sample data
        {/if}
      </button>
      
      <!-- Status message -->
      {#if sampleDataError}
        <span class="text-error text-sm">Error: {sampleDataError}</span>
      {/if}
      {#if sampleDataSuccess}
        <span class="text-success text-sm">✓ Sample data loaded successfully</span>
      {/if}
    </div>
  </div>

  <!-- Footer -->
  <div class="border-t border-base-300 pt-8">
    <p class="text-sm text-base-content/60 text-center">
      Made with 
      <img
        src="/src/assets/images/axolotl.svg"
        alt="axolotl emoji"
        class="inline w-5 h-5 mx-1"
      /> 
      by 
      <a href="https://github.com/CtrlSpice" 
         class="link link-primary" 
         target="_blank" 
         rel="noopener noreferrer">
        Mila Ardath
      </a>
      , with Artwork by 
      <a href="https://cbatesonart.artstation.com/" 
         class="link link-primary" 
         target="_blank" 
         rel="noopener noreferrer">
        Chelsey Bateson
      </a>
    </p>
  </div>
</div>