<script lang="ts">
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import { evalsEnabled } from '@/state/features'
  import type { ImportFailure } from '@/types/import-types'

  let {
    importFailure = null,
    onViewIssue,
  }: {
    importFailure?: ImportFailure | null
    onViewIssue?: (event: MouseEvent) => void
  } = $props()
  const workspace = $derived(
    $evalsEnabled ? import('./EvalsWorkspace.svelte') : null
  )
</script>

{#if workspace}
  {#await workspace}
    <p class="p-6 text-sm" role="status">Opening Evals…</p>
  {:then module}
    <module.default {importFailure} {onViewIssue} />
  {:catch}
    <p class="p-6 text-sm text-papaya" role="alert">
      Could not load Evals. Reload the page to try again.
    </p>
  {/await}
{:else}
  <PageLayout
    items={[]}
    selectedID={null}
    drawerID="signal-drawer"
    drawerLabel="Evals"
  >
    {#snippet main()}
      <div class="p-8">
        <h1 class="text-xl">Evals is not enabled</h1>
        <p class="mt-3 text-sm">
          Enable Evals in your local feature flags to open this experiment.
        </p>
      </div>
    {/snippet}
  </PageLayout>
{/if}
