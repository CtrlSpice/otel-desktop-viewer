<script module lang="ts">
  export type InkPickerOption = {
    id: string
    name: string
    english: string
    swatches: readonly { id: string; color: string }[]
  }
</script>

<script lang="ts">
  import { tick, type Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    HeartIcon,
    PaintBrush01Icon,
    PencilEdit02Icon,
    RefreshIcon,
    Tick02Icon,
  } from '@hugeicons/core-free-icons'
  import FilterPopover from './Toolbar/FilterPopover.svelte'
  import ListFilter from './Search/ListFilter.svelte'

  let {
    label,
    options,
    value,
    mode = 'ink',
    onchange,
    onreset,
    automatic = false,
    triggerColor,
    compactTrigger = false,
    editTrigger = false,
    unavailable = new Set<string>(),
    shadeControls,
    onOpenChange,
  }: {
    label: string
    options: readonly InkPickerOption[]
    value?: string
    mode?: 'ink' | 'shade'
    onchange: (value: string) => void
    onreset?: () => void
    automatic?: boolean
    triggerColor?: string
    compactTrigger?: boolean
    editTrigger?: boolean
    unavailable?: ReadonlySet<string>
    shadeControls?: Snippet<[option: InkPickerOption, close: () => void]>
    onOpenChange?: (open: boolean) => void
  } = $props()

  let open = $state(false)
  let search = $state('')
  let activeShade = $state<string>()
  let hoveredShade = $state<string>()
  let optionsList = $state<HTMLDivElement>()
  const dotGrid = $derived(mode === 'shade' && !shadeControls)
  const shadeLabel = $derived.by(() => {
    const id = hoveredShade ?? activeShade ?? value
    for (const ink of options) {
      const index = ink.swatches.findIndex(swatch => swatch.id === id)
      if (index >= 0) return `${ink.name} · ${index + 1}`
    }
    return ''
  })
  const query = $derived(search.trim().toLowerCase().replaceAll('-', ' '))
  const visible = $derived(
    options.filter(option =>
      `${option.name} ${option.english}`
        .toLowerCase()
        .replaceAll('-', ' ')
        .includes(query)
    )
  )
  const tabbableShade = $derived(
    visible.some(ink =>
      ink.swatches.some(
        swatch => swatch.id === activeShade && !unavailable.has(swatch.color)
      )
    )
      ? activeShade
      : visible
          .flatMap(ink => ink.swatches)
          .find(swatch => !unavailable.has(swatch.color))?.id
  )

  function reveal(button: HTMLElement, centre = false) {
    if (!optionsList) return
    const item = button.getBoundingClientRect()
    const list = optionsList.getBoundingClientRect()
    if (centre)
      optionsList.scrollTop +=
        item.top - list.top - (optionsList.clientHeight - item.height) / 2
    else if (item.top < list.top) optionsList.scrollTop += item.top - list.top
    else if (item.bottom > list.bottom)
      optionsList.scrollTop += item.bottom - list.bottom
  }

  async function handleOpenChange(next: boolean) {
    open = next
    onOpenChange?.(next)
    if (!next) {
      search = ''
      hoveredShade = undefined
      return
    }
    activeShade = value
    await tick()
    if (!open) return
    const button =
      optionsList?.querySelector<HTMLElement>(
        '[data-ink-slider][data-selected="true"], [data-ink-choice][aria-pressed="true"]:not(:disabled)'
      ) ??
      optionsList?.querySelector<HTMLElement>(
        '[data-ink-slider]:not(:disabled), [data-ink-choice]:not(:disabled)'
      )
    if (button) {
      activeShade = button.dataset.inkChoice
      button.focus({ preventScroll: true })
      reveal(button, true)
    }
  }

  function navigate(event: KeyboardEvent) {
    if (
      ![
        'ArrowLeft',
        'ArrowRight',
        'ArrowUp',
        'ArrowDown',
        'Home',
        'End',
      ].includes(event.key)
    )
      return
    const current = event.currentTarget
    if (!(current instanceof HTMLElement) || !optionsList) return
    event.preventDefault()
    hoveredShade = undefined
    const edge = event.key === 'Home' || event.key === 'End'
    const wholeGrid = edge && event.ctrlKey
    const horizontal =
      edge || event.key === 'ArrowLeft' || event.key === 'ArrowRight'
    const forward =
      event.key === 'End' ||
      event.key === 'ArrowRight' ||
      event.key === 'ArrowDown'
    const axis = horizontal ? 'left' : 'top'
    const cross = horizontal ? 'top' : 'left'
    const origin = current.getBoundingClientRect()
    const candidates = Array.from(
      optionsList.querySelectorAll<HTMLButtonElement>(
        '[data-ink-choice]:not(:disabled)'
      )
    )
      .map(button => ({ button, rect: button.getBoundingClientRect() }))
      .filter(
        ({ rect }) =>
          wholeGrid ||
          (Math.abs(rect[cross] - origin[cross]) < 1 &&
            (edge ||
              (forward
                ? rect[axis] > origin[axis] + 1
                : rect[axis] < origin[axis] - 1)))
      )
      .sort((a, b) =>
        wholeGrid
          ? a.rect.top - b.rect.top || a.rect.left - b.rect.left
          : a.rect[axis] - b.rect[axis]
      )
    const next = (edge === forward ? candidates.at(-1) : candidates[0])?.button
    if (!next) return
    activeShade = next.dataset.inkChoice
    next.focus({ preventScroll: true })
    reveal(next)
  }
</script>

<FilterPopover
  {label}
  compact
  triggerClass={triggerColor && !editTrigger
    ? `ink-picker-trigger--dot ${compactTrigger ? 'ink-picker-trigger--compact' : ''} item-state tooltip tooltip-right`
    : 'header-icon-button tooltip tooltip-left'}
  popoverClass={`ink-picker-popover ${dotGrid ? 'ink-picker--grid' : ''}`}
  showHeader={false}
  onOpenChange={handleOpenChange}
>
  {#snippet icon()}
    {#if editTrigger}
      <HugeiconsIcon
        icon={PencilEdit02Icon}
        size={16}
        strokeWidth={1.5}
        aria-hidden="true"
      />
    {:else if triggerColor}
      <span
        class="size-2 shrink-0 rounded-full"
        style:background-color={triggerColor}
        aria-hidden="true"
      ></span>
    {:else}
      <HugeiconsIcon
        icon={PaintBrush01Icon}
        size={16}
        strokeWidth={1.5}
        aria-hidden="true"
      />
    {/if}
  {/snippet}
  {#snippet children(close)}
    {#if open}
      {#if !dotGrid}<div class="ink-picker__measure" aria-hidden="true">
          {#each options as option (option.id)}<span
              >{option.name} ({option.english})</span
            >{/each}
          <div
            class="ink-picker__measure-dots"
            class:ink-picker__measure-dots--sliders={!!shadeControls}
          ></div>
        </div>{/if}
      <ListFilter
        bind:value={search}
        label="Find ink"
        placeholder="Find an ink…"
        onkeydown={event => {
          if (mode === 'shade' && event.key === 'ArrowDown' && visible.length) {
            event.preventDefault()
            const first = optionsList?.querySelector<HTMLElement>(
              '[data-ink-slider]:not(:disabled), [data-ink-choice]:not(:disabled)'
            )
            if (first) {
              first.focus({ preventScroll: true })
              reveal(first)
            }
          }
        }}
      >
        {#snippet children(filterToggle, filterField)}
          <div class="mb-2 flex shrink-0 items-center justify-end gap-2">
            {#if dotGrid}<span
                class="min-w-0 flex-1 truncate text-sm text-subtle"
                title={shadeLabel}>{shadeLabel}</span
              >{/if}
            {@render filterToggle()}
            {#if onreset}
              <button
                type="button"
                class="btn btn-ghost btn-circle btn-sm tooltip tooltip-left"
                aria-label="Reset to automatic colour"
                data-tip="Reset to automatic colour"
                disabled={automatic}
                onclick={() => {
                  onreset?.()
                  close()
                }}
              >
                <HugeiconsIcon
                  icon={RefreshIcon}
                  size={16}
                  strokeWidth={1.5}
                  aria-hidden="true"
                />
              </button>
            {/if}
          </div>
          {@render filterField()}
        {/snippet}
      </ListFilter>
      <div
        class="ink-picker-options anchor-popover__body"
        class:count-palette-options={mode === 'ink'}
        class:ink-picker-options--grid={dotGrid}
        bind:this={optionsList}
      >
        {#each visible as option (option.id)}
          {#if mode === 'ink'}
            <button
              type="button"
              class="item-state item-state--menu flex w-full cursor-pointer items-center gap-3 px-3 py-2 text-left font-sans text-sm font-normal"
              data-ink-choice={option.id}
              aria-pressed={option.id === value}
              onclick={() => {
                onchange(option.id)
                close()
              }}
            >
              <span class="min-w-0 flex-1"
                >{option.name}
                <span class="text-subtle">({option.english})</span></span
              >
              <span class="flex shrink-0 items-center gap-2" aria-hidden="true">
                {#each option.swatches as swatch}<span
                    class="size-2 shrink-0 rounded-full"
                    style:background-color={swatch.color}
                  ></span>{/each}
              </span>
              <span class="size-4 shrink-0" aria-hidden="true"
                >{#if option.id === value}<HugeiconsIcon
                    icon={Tick02Icon}
                    size={16}
                    strokeWidth={1.5}
                  />{/if}</span
              >
            </button>
          {:else}
            <div
              class="ink-picker__shade-row"
              class:ink-picker__shade-row--grid={dotGrid}
              role="group"
              aria-label={`${option.name} (${option.english})`}
            >
              {#if !dotGrid}<span class="min-w-0"
                  >{option.name}
                  <span class="text-subtle">({option.english})</span></span
                >{/if}
              {#if shadeControls}
                {@render shadeControls(option, close)}
              {:else}
                <div
                  class="flex shrink-0 items-center"
                  class:flex-col-reverse={dotGrid}
                >
                  {#each option.swatches as swatch, column (swatch.id)}
                    {@const taken = unavailable.has(swatch.color)}
                    <button
                      type="button"
                      class="ink-picker__shade item-state item-state--menu"
                      data-ink-choice={swatch.id}
                      aria-label={`${option.name} (${option.english}), shade ${column + 1}, ${swatch.color}${taken ? ', already assigned' : ''}`}
                      title={`${option.name} · ${column + 1} · ${swatch.color}${taken ? ' · Already assigned' : ''}`}
                      disabled={taken}
                      aria-pressed={swatch.id === value}
                      tabindex={swatch.id === tabbableShade ? 0 : -1}
                      onfocus={() => {
                        activeShade = swatch.id
                      }}
                      onpointerenter={() => (hoveredShade = swatch.id)}
                      onpointerleave={() => (hoveredShade = undefined)}
                      onkeydown={navigate}
                      onclick={() => {
                        onchange(swatch.id)
                        close()
                      }}
                    >
                      <span
                        class="size-2 rounded-full"
                        style:background-color={taken
                          ? 'var(--color-base-300)'
                          : swatch.color}
                        aria-hidden="true"
                      ></span>
                    </button>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
        {:else}
          <p class="px-3 py-2 text-sm text-subtle">No matching inks.</p>
        {/each}
      </div>
    {/if}
  {/snippet}
  {#snippet footer()}
    <a
      class="link mx-auto inline-flex items-center gap-2 text-sm text-subtle"
      href="https://madoromi.moonwit.ch/"
      target="_blank"
      rel="noreferrer"
    >
      <span>Colours by Madoromi</span><HugeiconsIcon
        icon={HeartIcon}
        size={16}
        strokeWidth={1.5}
        class="shrink-0 text-(--color-sakura)"
        aria-hidden="true"
      />
    </a>
  {/snippet}
</FilterPopover>

<style lang="postcss">
  @reference '../../app.css';
  :global(.ink-picker-trigger--dot) {
    @apply inline-flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-full border-0 p-0;
  }
  :global(.ink-picker-trigger--compact) {
    @apply size-6;
  }
  :global(.ink-picker-trigger--dot[aria-expanded='true']) {
    --item-idle-bg: var(--item-selected-bg);
  }
  .ink-picker-options {
    max-height: min(55vh, 22rem);
    container-type: inline-size;
  }
  :global(.filter-popover.ink-picker-popover) {
    width: max-content;
    min-width: min(24rem, calc(100vw - 1.5rem));
  }
  :global(.filter-popover.ink-picker--grid) {
    min-width: 0;
  }
  .ink-picker-options--grid {
    display: grid;
    grid-template-columns: repeat(16, 1.5rem);
    row-gap: calc(2 * var(--header-gap));
    width: 24rem;
    max-height: none;
  }
  /* All names share one invisible grid cell. Its intrinsic width includes the
     widest rendered label, even while search hides that ink from the list. */
  .ink-picker__measure {
    display: grid;
    grid-template-columns: max-content auto;
    gap: 0.75rem;
    height: 0;
    flex-shrink: 0;
    padding-inline: 0.75rem;
    overflow: hidden;
    visibility: hidden;
    font-size: var(--text-sm);
  }
  .ink-picker__measure > span {
    grid-area: 1 / 1;
    white-space: nowrap;
  }
  .ink-picker__measure-dots {
    grid-area: 1 / 2;
    width: 8.25rem;
  }
  .ink-picker__measure-dots--sliders {
    width: 14rem;
  }
  .ink-picker__shade-row {
    @apply grid items-center gap-3 px-3 py-2 text-sm font-normal;
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .ink-picker__shade {
    @apply flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-full;
  }
  .ink-picker__shade-row--grid {
    display: block;
    padding: 0;
  }
  .ink-picker__shade:disabled {
    cursor: not-allowed;
  }
  @container (max-width: 26rem) {
    .ink-picker__shade-row:not(.ink-picker__shade-row--grid) {
      grid-template-columns: minmax(0, 1fr);
      gap: 0.25rem;
    }
  }
  @media (max-width: 27rem) {
    .ink-picker-options--grid {
      grid-template-columns: repeat(8, 1.5rem);
      width: 12rem;
    }
  }
</style>
