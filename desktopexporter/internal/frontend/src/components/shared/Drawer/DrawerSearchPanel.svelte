<script lang="ts">
  import { ArrowUpIcon, SortingIcon } from '@/icons'
  import DateTimeFilter from '@/components/shared/Toolbar/DateTimeFilter.svelte'
  import SearchEditor from '@/components/shared/Search/SearchEditor.svelte'
  import {
    createPopoverID,
    setupAnchorPopover,
  } from '@/components/shared/utils/anchor-popover'
  import type { SearchResultEvent } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'

  import type { SortOption } from '@/contexts/signal-list-page.svelte'

  type DrawerSearchPanelSegment = 'full' | 'toolbar' | 'search'

  type Props = {
    /** `toolbar` = sort/time/refresh · `search` = editor · `full` = both */
    segment?: DrawerSearchPanelSegment
    signal: 'traces' | 'metrics' | 'logs'
    sortOptions: SortOption[]
    sortValue: string
    sortDirection: 'asc' | 'desc'
    onSortChange?: (value: string, direction: 'asc' | 'desc') => void
    onSearchResults?: (event: SearchResultEvent) => void
    onSearchError?: (error: string | null) => void
    onSearchReady?: (api: SearchEditorAPI) => void
  }

  let {
    segment = 'full',
    signal,
    sortOptions,
    sortValue,
    sortDirection,
    onSortChange,
    onSearchResults,
    onSearchError,
    onSearchReady,
  }: Props = $props()

  let sortPopoverEl = $state<HTMLDivElement | null>(null)
  let sortTriggerEl = $state<HTMLButtonElement | null>(null)
  let sortPopoverOpen = $state(false)
  let focusedSortIndex = $state(0)
  let pendingMenuFocus: 'selected' | 'first' | 'last' = 'selected'

  const sortPopoverID = createPopoverID('sort-popover')

  let currentSortLabel = $derived(
    sortOptions.find(o => o.value === sortValue)?.label ?? 'Sort'
  )

  let sortAriaLabel = $derived(
    `Sort by ${currentSortLabel}, ${sortDirection === 'asc' ? 'ascending' : 'descending'}`
  )

  $effect(() => {
    const popover = sortPopoverEl
    const trigger = sortTriggerEl
    if (!popover || !trigger) return
    return setupAnchorPopover({
      popover,
      trigger,
      anchor: 'below-end',
      onOpenChange: open => {
        sortPopoverOpen = open
        if (open) {
          const target = pendingMenuFocus
          pendingMenuFocus = 'selected'
          queueMicrotask(() => focusSortItem(target))
        }
      },
    })
  })

  function nextDirection(opt: SortOption): 'asc' | 'desc' {
    if (opt.value !== sortValue) return opt.defaultDirection ?? 'asc'
    return sortDirection === 'asc' ? 'desc' : 'asc'
  }

  function selectSort(value: string, dir: 'asc' | 'desc') {
    onSortChange?.(value, dir)
    sortPopoverEl?.hidePopover()
    sortTriggerEl?.focus()
  }

  function sortMenuItems(): HTMLButtonElement[] {
    return Array.from(
      sortPopoverEl?.querySelectorAll<HTMLButtonElement>(
        '[role="menuitemradio"]'
      ) ?? []
    )
  }

  function focusSortItem(target: 'selected' | 'first' | 'last' | number) {
    const menuItems = sortMenuItems()
    if (menuItems.length === 0) {
      sortTriggerEl?.focus()
      return
    }
    const selectedIndex = Math.max(
      0,
      sortOptions.findIndex(option => option.value === sortValue)
    )
    const index =
      typeof target === 'number'
        ? target
        : target === 'first'
          ? 0
          : target === 'last'
            ? menuItems.length - 1
            : selectedIndex
    focusedSortIndex = index
    menuItems[index]?.focus()
  }

  function openSortMenu(target: 'selected' | 'first' | 'last') {
    pendingMenuFocus = target
    if (sortPopoverOpen) {
      focusSortItem(target)
    } else {
      sortPopoverEl?.showPopover()
    }
  }

  function closeSortMenu() {
    sortPopoverEl?.hidePopover()
    sortTriggerEl?.focus()
  }

  function handleSortTriggerKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      openSortMenu('first')
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      openSortMenu('last')
    }
  }

  function handleSortMenuKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      closeSortMenu()
      return
    }

    const menuItems = sortMenuItems()
    const current = (
      event.target as Element | null
    )?.closest<HTMLButtonElement>('[role="menuitemradio"]')
    const currentIndex = current ? menuItems.indexOf(current) : -1
    if (currentIndex < 0 || menuItems.length === 0) return

    let nextIndex: number
    if (event.key === 'ArrowDown') {
      nextIndex = (currentIndex + 1) % menuItems.length
    } else if (event.key === 'ArrowUp') {
      nextIndex = (currentIndex - 1 + menuItems.length) % menuItems.length
    } else if (event.key === 'Home') {
      nextIndex = 0
    } else if (event.key === 'End') {
      nextIndex = menuItems.length - 1
    } else if (
      event.key.length === 1 &&
      !event.altKey &&
      !event.ctrlKey &&
      !event.metaKey
    ) {
      const prefix = event.key.toLocaleLowerCase()
      nextIndex = menuItems.findIndex((item, index) => {
        const wrappedIndex = (currentIndex + index + 1) % menuItems.length
        return menuItems[wrappedIndex].textContent
          ?.trim()
          .toLocaleLowerCase()
          .startsWith(prefix)
      })
      if (nextIndex < 0) return
      nextIndex = (currentIndex + nextIndex + 1) % menuItems.length
    } else {
      return
    }

    event.preventDefault()
    focusSortItem(nextIndex)
  }

  function handleSortPopoverFocusout(event: FocusEvent) {
    const next = event.relatedTarget
    if (next instanceof Node && sortPopoverEl?.contains(next)) return
    sortPopoverEl?.hidePopover()
  }
</script>

<div class="drawer-search-panel">
  {#if segment === 'full' || segment === 'toolbar'}
    <!-- Toolbar row: time · sort -->
    <div
      class="drawer-search-panel__toolbar-row"
      role="toolbar"
      aria-label="List controls"
    >
      <DateTimeFilter
        class="drawer-header-btn drawer-header-btn--inactive shrink-0 tooltip tooltip-bottom"
      />

      <button
        bind:this={sortTriggerEl}
        type="button"
        class="drawer-header-btn drawer-header-btn--inactive shrink-0 tooltip tooltip-bottom"
        popovertarget={sortPopoverID}
        aria-expanded={sortPopoverOpen}
        aria-haspopup="menu"
        aria-controls={sortPopoverID}
        aria-label={sortAriaLabel}
        data-tip="Sort"
        onkeydown={handleSortTriggerKeydown}
      >
        <SortingIcon class="h-[17px] w-[17px] shrink-0" />
      </button>

      <div
        bind:this={sortPopoverEl}
        popover="auto"
        id={sortPopoverID}
        class="anchor-popover anchor-popover--anchored anchor-popover--menu"
        onfocusout={handleSortPopoverFocusout}
      >
        <ul
          class="anchor-popover-menu"
          role="menu"
          aria-label="Sort by"
          onkeydown={handleSortMenuKeydown}
        >
          {#each sortOptions as opt, index (opt.value)}
            <li role="none">
              <button
                type="button"
                role="menuitemradio"
                aria-checked={opt.value === sortValue}
                tabindex={index === focusedSortIndex ? 0 : -1}
                class="anchor-popover-menu__option {opt.value === sortValue
                  ? 'anchor-popover-menu__option--active'
                  : ''}"
                onfocus={() => (focusedSortIndex = index)}
                onclick={() => selectSort(opt.value, nextDirection(opt))}
              >
                <span>{opt.label}</span>
                {#if opt.value === sortValue}
                  <ArrowUpIcon
                    class="anchor-popover-menu__option-icon {sortDirection ===
                    'desc'
                      ? 'rotate-180'
                      : ''}"
                    aria-hidden="true"
                  />
                {/if}
              </button>
            </li>
          {/each}
        </ul>
      </div>
    </div>
  {/if}

  {#if segment === 'full' || segment === 'search'}
    <SearchEditor
      {signal}
      variant="drawer"
      {onSearchResults}
      {onSearchError}
      onReady={onSearchReady}
    />
  {/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .drawer-search-panel {
    @apply flex w-full min-w-0 flex-col gap-2;
  }

  .drawer-search-panel__toolbar-row {
    @apply flex min-w-0 items-center justify-end gap-2;
  }
</style>
