<script module lang="ts">
  export function displaySessionName(filename: string): string {
    return filename.replace(/\.duckdb$/i, '')
  }

  export function formatBytes(bytes: number): string {
    if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    let value = bytes
    let index = 0
    while (value >= 1000 && index < units.length - 1) {
      value /= 1000
      index++
    }
    let rounded =
      index === 0 || value >= 10
        ? Math.round(value)
        : Math.round(value * 10) / 10
    if (rounded >= 1000 && index < units.length - 1) {
      rounded /= 1000
      index++
    }
    const formatted =
      index === 0 || rounded >= 10
        ? String(Math.round(rounded))
        : rounded.toFixed(1)
    return `${formatted} ${units[index]}`
  }
</script>

<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Add01Icon, Database01Icon } from '@hugeicons/core-free-icons'
  import FilterPopover from './FilterPopover.svelte'
  import { telemetryAPI, isAbortError } from '@/services/telemetry-service'
  import { getDataChanges } from '@/contexts/data-changes.svelte'
  import { buildSearch, navigate, readRoute, sessionSwitchHref } from '@/route'
  import type { DatabaseList } from '@/types/api-types'

  const changes = getDataChanges()

  let sessions = $state<DatabaseList | null>(null)
  let newName = $state('')
  let busy = $state(false)
  let errorMessage = $state('')
  let nameInput = $state<HTMLInputElement>()
  let listController: AbortController | null = null
  let actionController: AbortController | null = null
  let destroyed = false

  async function refreshList(surfaceErrors = false) {
    listController?.abort()
    const controller = new AbortController()
    listController = controller
    try {
      const next = await telemetryAPI.listDatabases(controller.signal)
      if (!destroyed && !controller.signal.aborted) {
        sessions = next
      }
    } catch (err) {
      if (destroyed || controller.signal.aborted || isAbortError(err)) return
      if (surfaceErrors) {
        errorMessage = err instanceof Error ? err.message : String(err)
      }
      // Initial probe stays quiet in non-managed or test environments without the session RPC.
    }
  }

  onMount(() => {
    void refreshList(false)
  })

  onDestroy(() => {
    destroyed = true
    listController?.abort()
    actionController?.abort()
  })

  function afterSwitch(next: DatabaseList, close: () => void) {
    sessions = next
    errorMessage = ''
    newName = ''
    const currentRoute = readRoute()
    const currentHref =
      currentRoute.path + buildSearch(currentRoute.query) + window.location.hash
    const nextHref = sessionSwitchHref(currentRoute)
    if (nextHref !== currentHref) {
      navigate(nextHref, 'replace')
    }
    changes?.notifySessionChanged()
    close()
  }

  async function selectDatabase(name: string, close: () => void) {
    if (busy || name === sessions?.active) {
      close()
      return
    }
    actionController?.abort()
    const controller = new AbortController()
    actionController = controller
    busy = true
    errorMessage = ''
    try {
      const next = await telemetryAPI.openDatabase(name, controller.signal)
      if (destroyed || controller.signal.aborted) return
      afterSwitch(next, close)
    } catch (err) {
      if (destroyed || controller.signal.aborted || isAbortError(err)) return
      errorMessage = err instanceof Error ? err.message : String(err)
    } finally {
      if (!destroyed && actionController === controller) {
        actionController = null
        busy = false
      }
    }
  }

  async function createDatabase(event: SubmitEvent, close: () => void) {
    event.preventDefault()
    const trimmed = newName.trim()
    if (busy || !trimmed) return
    actionController?.abort()
    const controller = new AbortController()
    actionController = controller
    busy = true
    errorMessage = ''
    try {
      const next = await telemetryAPI.newDatabase(trimmed, controller.signal)
      if (destroyed || controller.signal.aborted) return
      afterSwitch(next, close)
    } catch (err) {
      if (destroyed || controller.signal.aborted || isAbortError(err)) return
      errorMessage = err instanceof Error ? err.message : String(err)
    } finally {
      if (!destroyed && actionController === controller) {
        actionController = null
        busy = false
      }
    }
  }

  function handleOpenChange(open: boolean) {
    if (open) {
      errorMessage = ''
      if (nameInput && !nameInput.disabled) {
        nameInput.focus({ preventScroll: true })
      }
      void refreshList(true)
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
            bind:this={nameInput}
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
