<script lang="ts">
  import { onMount } from 'svelte';
  
  let count = 0;
  let isDarkMode = true; // Default to dark mode (Monokai style)
  
  function increment() {
    count += 1;
  }
  
  function toggleTheme() {
    isDarkMode = !isDarkMode;
    updateTheme();
  }
  
  function updateTheme() {
    if (isDarkMode) {
      document.documentElement.setAttribute('data-theme', 'default-dark');
    } else {
      document.documentElement.setAttribute('data-theme', 'default-light');
    }
    localStorage.setItem('theme', isDarkMode ? 'dark' : 'light');
  }
  
  onMount(() => {
    // Load saved theme preference
    const savedTheme = localStorage.getItem('theme');
    if (savedTheme) {
      isDarkMode = savedTheme === 'dark';
    }
    updateTheme();
  });
</script>

<main class="min-h-screen bg-base-200 transition-colors duration-300">
  <div class="container mx-auto p-8">
    <!-- Theme Toggle -->
    <div class="fixed top-4 right-4 z-50">
      <button 
        class="btn btn-circle btn-ghost bg-base-100/80 backdrop-blur-sm shadow-lg hover:shadow-xl transition-all duration-300" 
        on:click={toggleTheme}
        title="Toggle theme"
      >
        {#if isDarkMode}
          <svg class="w-6 h-6 text-accent" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd" d="M10 2a1 1 0 011 1v1a1 1 0 11-2 0V3a1 1 0 011-1zm4 8a4 4 0 11-8 0 4 4 0 018 0zm-.464 4.95l.707.707a1 1 0 001.414-1.414l-.707-.707a1 1 0 00-1.414 1.414zm2.12-10.607a1 1 0 010 1.414l-.706.707a1 1 0 11-1.414-1.414l.707-.707a1 1 0 011.414 0zM17 11a1 1 0 100-2h-1a1 1 0 100 2h1zm-7 4a1 1 0 011 1v1a1 1 0 11-2 0v-1a1 1 0 011-1zM5.05 6.464A1 1 0 106.465 5.05l-.708-.707a1 1 0 00-1.414 1.414l.707.707zm1.414 8.486l-.707.707a1 1 0 01-1.414-1.414l.707-.707a1 1 0 011.414 1.414zM4 11a1 1 0 100-2H3a1 1 0 000 2h1z" clip-rule="evenodd"></path>
          </svg>
        {:else}
          <svg class="w-6 h-6 text-accent" fill="currentColor" viewBox="0 0 20 20">
            <path d="M17.293 13.293A8 8 0 016.707 2.707a8.001 8.001 0 1010.586 10.586z"></path>
          </svg>
        {/if}
      </button>
    </div>

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

