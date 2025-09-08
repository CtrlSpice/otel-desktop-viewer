<script lang="ts">
  import ThemeToggle from './components/ThemeToggle.svelte';
  
  let count = 0;
  
  function increment() {
    count += 1;
  }
</script>

<main class="min-h-screen bg-base-200 transition-colors duration-300">
  <div class="container mx-auto p-8">
    <ThemeToggle />

    <div class="text-center py-12">
      <h1 class="text-5xl font-bold text-primary mb-4 tracking-tight">
        OTel Desktop Viewer
      </h1>
      <p class="text-lg text-base-content/70 mb-12">
        OpenTelemetry observability made beautiful
      </p>
      
      <div class="card w-96 bg-base-100 shadow-2xl mx-auto border border-accent/20 hover:shadow-accent/10 transition-all duration-300">
        <div class="card-body">
          <h2 class="card-title text-accent mb-2">Welcome to v2!</h2>
          <p class="text-base-content/80 mb-4">Built with Svelte + Vite + Custom Themes</p>
          
          <div class="card-actions justify-center">
            <button 
              class="btn btn-primary btn-wide shadow-lg hover:shadow-primary/25 transition-all duration-300" 
              on:click={increment}
            >
              <span class="mr-2">🚀</span>
              Count: {count}
            </button>
          </div>
        </div>
      </div>
      
      <div class="mt-12 grid grid-cols-2 md:grid-cols-3 gap-4 max-w-2xl mx-auto">
        <div class="stat bg-base-100/50 backdrop-blur-sm rounded-lg border border-accent/10">
          <div class="stat-figure text-success">✅</div>
          <div class="stat-title text-xs">Vite</div>
          <div class="stat-value text-sm">Ready</div>
        </div>
        <div class="stat bg-base-100/50 backdrop-blur-sm rounded-lg border border-accent/10">
          <div class="stat-figure text-success">✅</div>
          <div class="stat-title text-xs">Svelte</div>
          <div class="stat-value text-sm">Active</div>
        </div>
        <div class="stat bg-base-100/50 backdrop-blur-sm rounded-lg border border-accent/10">
          <div class="stat-figure text-success">✅</div>
          <div class="stat-title text-xs">TypeScript</div>
          <div class="stat-value text-sm">Working</div>
        </div>
        <div class="stat bg-base-100/50 backdrop-blur-sm rounded-lg border border-accent/10">
          <div class="stat-figure text-success">✅</div>
          <div class="stat-title text-xs">Tailwind</div>
          <div class="stat-value text-sm">Styled</div>
        </div>
        <div class="stat bg-base-100/50 backdrop-blur-sm rounded-lg border border-accent/10">
          <div class="stat-figure text-success">✅</div>
          <div class="stat-title text-xs">DaisyUI</div>
          <div class="stat-value text-sm">Themed</div>
        </div>
        <div class="stat bg-base-100/50 backdrop-blur-sm rounded-lg border border-accent/10">
          <div class="stat-figure text-primary">🎨</div>
          <div class="stat-title text-xs">Themes</div>
          <div class="stat-value text-sm">Custom</div>
        </div>
      </div>
    </div>
  </div>
</main>

<style>
  :global(.btn) {
    transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  }

  :global(.btn:hover) {
    transform: translateY(-1px);
  }

  :global(.card) {
    backdrop-filter: blur(10px);
    transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  }

  :global(.card:hover) {
    transform: translateY(-2px);
  }

  /* Clean solid backgrounds */
  :global([data-theme="default-dark"]) {
    background: #1a1a1a;
  }

  :global([data-theme="default-light"]) {
    background: #f8f9fa;
  }
</style>

