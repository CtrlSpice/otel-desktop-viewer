<script module lang="ts">
  export function displaySessionName(filename: string): string {
    return filename.replace(/\.duckdb$/i, '')
  }

  export function formatBytes(bytes: number): string {
    if (!Number.isFinite(bytes) || bytes < 1024)
      return `${Math.max(0, bytes)} B`
    const units = ['KB', 'MB', 'GB', 'TB']
    let value = bytes / 1024
    let index = 0
    while (value >= 1024 && index < units.length - 1) {
      value /= 1024
      index++
    }
    return `${value >= 10 ? Math.round(value) : value.toFixed(1)} ${units[index]}`
  }

  function topLevelRouteFor(path: string): string | null {
    for (const base of ['/traces', '/metrics', '/logs', '/evals']) {
      if (path.startsWith(`${base}/`)) return base
    }
    return null
  }
</script>

<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Add01Icon, Database01Icon } from '@hugeicons/core-free-icons'
  import FilterPopover from './FilterPopover.svelte'
  import { telemetryAPI, isAbortError } from '@/services/telemetry-service'
  import { getDataChanges } from '@/contexts/data-changes.svelte'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import { navigate } from '@/route'
  import type { DatabaseList } from '@/types/api-types'

  const changes = getDataChanges()
  const routeContext = getRouteContext()

  let sessions = $state<DatabaseList | null>(null)
  let newName = $state('')
  let busy = $state(false)
  let errorMessage = $state('')
  let listController: AbortController | null = null

  async function refreshList() {
    listController?.abort()
    const controller = new AbortController()
    listController = controller
    try {
      const next = await telemetryAPI.listDatabases(controller.signal)
      if (!controller.signal.aborted) {
        sessions = next
      }
    } catch (err) {
      if (isAbortError(err)) return
      // Non-managed or test environments without the session RPC stay quiet.
    }
  }

  onMount(() => {
    void refreshList()
  })

  onDestroy(() => {
    listController?.abort()
  })

  function afterSwitch(next: DatabaseList, close: () => void) {
    sessions = next
    errorMessage = ''
    newName = ''
    const topLevel = topLevelRouteFor(routeContext.route.path)
    if (topLevel) navigate(topLevel)
    changes?.refreshViews()
    close()
  }

  async function selectDatabase(name: string, close: () => void) {
    if (busy || name === sessions?.active) {
      close()
      return
    }
    busy = true
    errorMessage = ''
    try {
      const next = await telemetryAPI.openDatabase(name)
      afterSwitch(next, close)
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : String(err)
    } finally {
      busy = false
    }
  }

  async function createDatabase(event: SubmitEvent, close: () => void) {
    event.preventDefault()
    const trimmed = newName.trim()
    if (busy || !trimmed) return
    busy = true
    errorMessage = ''
    try {
      const next = await telemetryAPI.newDatabase(trimmed)
      afterSwitch(next, close)
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : String(err)
    } finally {
      busy = false
    }
  }

  function handleOpenChange(open: boolean) {
    if (open) {
      errorMessage = ''
      void refreshList()
    }
  }

  const activeLabel = $derived(
    sessions?.active ? displaySessionName(sessions.active) : 'Session'
  )
</script>

{#if sessions?.managed}
  <FilterPopover
    label={`Session: ${activeLabel}`}
    tooltip={`Active session: ${sessions.active}`}
    anchor="below-end"
    role="dialog"
    compact
    triggerClass="btn btn-ghost btn-sm rounded-full bg-base-200 gap-1.5 px-3 font-normal text-sm text-base-content tooltip tooltip-bottom"
    popoverClass="database-switcher-popover"
    showHeader={false}
    onOpenChange={handleOpenChange}
  >
    {#snippet icon()}
      <HugeiconsIcon
        icon={Database01Icon}
        size={16}
        strokeWidth={1.5}
        class="shrink-0 text-primary"
        aria-hidden="true"
      />
      <span class="max-w-32 truncate">{activeLabel}</span>
    {/snippet}
    {#snippet children(close)}
      <div class="database-switcher">
        <form
          class="database-switcher__create"
          onsubmit={event => void createDatabase(event, close)}
        >
          <input
            type="text"
            class="input input-xs input-bordered min-w-0 flex-1"
            aria-label="New session name"
            placeholder="New session name"
            bind:value={newName}
            disabled={busy}
          />
          <button
            type="submit"
            class="btn btn-xs btn-primary shrink-0 gap-1"
            disabled={busy || !newName.trim()}
          >
            <HugeiconsIcon
              icon={Add01Icon}
              size={14}
              strokeWidth={1.75}
              aria-hidden="true"
            />
            <span>New</span>
          </button>
        </form>

        {#if errorMessage}
          <p class="database-switcher__error" role="alert">{errorMessage}</p>
        {/if}

        <ul class="database-switcher__list" aria-label="Saved sessions">
          {#each sessions?.databases ?? [] as entry (entry.name)}
            <li>
              <button
                type="button"
                class="database-switcher__item item-state item-state--menu"
                aria-current={entry.active ? 'true' : undefined}
                disabled={busy}
                onclick={() => void selectDatabase(entry.name, close)}
              >
                <span class="min-w-0 flex-1 truncate">
                  {displaySessionName(entry.name)}
                </span>
                <span class="database-switcher__meta">
                  {formatBytes(entry.sizeBytes)}
                </span>
              </button>
            </li>
          {/each}
        </ul>
      </div>
    {/snippet}
  </FilterPopover>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  :global(.filter-popover.database-switcher-popover) {
    width: 17rem;
    padding: 0.625rem;
  }

  .database-switcher {
    @apply flex flex-col gap-2;
  }

  .database-switcher__create {
    @apply flex items-center gap-1.5;
  }

  .database-switcher__error {
    @apply rounded bg-error/10 px-2 py-1 text-xs text-error;
  }

  .database-switcher__list {
    @apply flex max-h-60 flex-col gap-0.5 overflow-y-auto;
  }

  .database-switcher__item {
    @apply flex min-h-8 w-full items-center justify-between gap-2 px-2 py-1 text-left text-sm;
  }

  .database-switcher__item[aria-current='true'] {
    background: var(--item-selected-bg);
    font-weight: 500;
  }

  .database-switcher__meta {
    @apply shrink-0 text-xs text-subtle tabular-nums;
  }
</style>
