<!-- TracesPage.svelte - Traces list and visualization page -->
<div class="flex flex-col items-center justify-center w-full overflow-y-auto">
  <div class="card bg-base-100 shadow-2xl my-16 min-w-[960px] p-8 w-4/5">
    <div class="card-header pb-0">
      <h1 class="text-2xl font-bold">Traces</h1>
      <div class="divider"></div>
    </div>

    <div class="card-body">
      <p class="text-base-content/80">
        This is where your traces will be displayed. Trace summaries will be
        displayed here.
      </p>

      <!-- Placeholder for traces content -->
      <div class="mt-8 p-8 bg-base-200 rounded-lg text-center">
        <p class="text-base-content/60">Traces content coming soon...</p>
        <p class="text-sm text-base-content/50 mt-2">
          Future: This will show trace list, and clicking a trace will navigate
          to /traces/id=123
        </p>
      </div>
    </div>
  </div>
</div>
