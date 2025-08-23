<script lang="ts">
  let count = 0;
  
  function increment() {
    count += 1;
  }
</script>

<main class="container mx-auto p-8">
  <div class="text-center">
    <h1 class="text-4xl font-bold text-primary mb-8">
      Hello OTel-Desktop-Viewer
    </h1>
    
    <div class="card w-96 bg-base-100 shadow-xl mx-auto">
      <div class="card-body">
        <h2 class="card-title">Welcome to Svelte + Vite + DaisyUI!</h2>
        <p>This is your v2 frontend running with the new tech stack.</p>
        
        <div class="card-actions justify-center mt-4">
          <button class="btn btn-primary" on:click={increment}>
            Count is {count}
          </button>
        </div>
      </div>
    </div>
    
    <div class="mt-8 text-sm text-gray-600">
      <p>✅ Vite is working</p>
      <p>✅ Svelte is working</p>
      <p>✅ TypeScript is working</p>
      <p>✅ Tailwind CSS is working</p>
      <p>✅ DaisyUI is working</p>
    </div>
  </div>
</main>

<style>
  main {
    min-height: 100vh;
    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  }
</style>

