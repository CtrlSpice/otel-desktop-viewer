<script module lang="ts">
  export type InkPickerOption = {
    id: string
    name: string
    english: string
    swatches: readonly { id: string; color: string }[]
  }
</script>

<script lang="ts">
  import { tick } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    HeartIcon,
    PaintBrush01Icon,
    Tick02Icon,
  } from '@hugeicons/core-free-icons'
  import FilterPopover from './Toolbar/FilterPopover.svelte'
  import FilterInput from './Search/FilterInput.svelte'
  import { readableTextColor } from '@/utils/chart-palette'

  let {
    label,
    options,
    value,
    mode = 'ink',
    onchange,
    onreset,
    automatic = false,
    triggerColor,
  }: {
    label: string
    options: readonly InkPickerOption[]
    value?: string
    mode?: 'ink' | 'shade'
    onchange: (value: string) => void
    onreset?: () => void
    automatic?: boolean
    triggerColor?: string
  } = $props()

  let open = $state(false)
  let search = $state('')
  let activeShade = $state<string>()
  let optionsList = $state<HTMLDivElement>()
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
    visible.some(ink => ink.swatches.some(swatch => swatch.id === activeShade))
      ? activeShade
      : visible[0]?.swatches[0]?.id
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
    if (!next) {
      search = ''
      return
    }
    activeShade = value
    await tick()
    if (!open) return
    const button =
      optionsList?.querySelector<HTMLButtonElement>(
        '[data-ink-choice][aria-pressed="true"]'
      ) ?? optionsList?.querySelector<HTMLButtonElement>('[data-ink-choice]')
    if (button) {
      activeShade = button.dataset.inkChoice
      button.focus({ preventScroll: true })
      reveal(button, true)
    }
  }

  function focusShade(row: number, column: number) {
    const swatch = visible[row]?.swatches[column]
    if (!swatch) return
    activeShade = swatch.id
    const button = optionsList?.querySelector<HTMLButtonElement>(
      `[data-ink-choice="${CSS.escape(swatch.id)}"]`
    )
    if (button) {
      button.focus({ preventScroll: true })
      reveal(button)
    }
  }

  function navigate(event: KeyboardEvent, row: number, column: number) {
    const last = visible[row]!.swatches.length - 1
    switch (event.key) {
      case 'ArrowLeft':
        column = Math.max(0, column - 1)
        break
      case 'ArrowRight':
        column = Math.min(last, column + 1)
        break
      case 'ArrowUp':
        row = Math.max(0, row - 1)
        break
      case 'ArrowDown':
        row = Math.min(visible.length - 1, row + 1)
        break
      case 'Home':
        column = 0
        if (event.ctrlKey) row = 0
        break
      case 'End':
        column = last
        if (event.ctrlKey) row = visible.length - 1
        break
      default:
        return
    }
    event.preventDefault()
    focusShade(row, column)
  }
</script>

<FilterPopover
  {label}
  compact
  triggerClass={triggerColor
    ? 'btn btn-circle btn-sm ink-picker-trigger--filled border-0 shadow-none font-normal tooltip tooltip-right'
    : 'header-icon-button tooltip tooltip-left'}
  triggerStyle={triggerColor
    ? `--btn-color: ${triggerColor}; --btn-fg: ${readableTextColor(triggerColor)}`
    : undefined}
  popoverClass={`ink-picker-popover ${mode === 'shade' ? 'ink-picker--shades' : ''}`}
  showHeader={false}
  onOpenChange={handleOpenChange}
>
  {#snippet icon()}
    <HugeiconsIcon
      icon={PaintBrush01Icon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />
  {/snippet}
  {#snippet children(close)}
    {#if open}
      <div class="ink-picker__measure" aria-hidden="true">
        {#each options as option (option.id)}<span
            >{option.name} ({option.english})</span
          >{/each}
        <div
          class="ink-picker__measure-dots"
          class:ink-picker__measure-dots--shades={mode === 'shade'}
        ></div>
      </div>
      <div class="mb-2 shrink-0">
        <FilterInput
          bind:value={search}
          label="Find ink"
          placeholder="Find an ink…"
          onkeydown={event => {
            if (
              mode === 'shade' &&
              event.key === 'ArrowDown' &&
              visible.length
            ) {
              event.preventDefault()
              focusShade(0, 0)
            }
          }}
        />
      </div>
      <div
        class="ink-picker-options anchor-popover__body"
        class:count-palette-options={mode === 'ink'}
        bind:this={optionsList}
      >
        {#each visible as option, row (option.id)}
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
                <span class="text-rp-subtle">({option.english})</span></span
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
              role="group"
              aria-label={`${option.name} (${option.english})`}
            >
              <span class="min-w-0"
                >{option.name}
                <span class="text-rp-subtle">({option.english})</span></span
              >
              <div class="flex shrink-0 items-center">
                {#each option.swatches as swatch, column (swatch.id)}
                  <button
                    type="button"
                    class="ink-picker__shade item-state item-state--menu"
                    data-ink-choice={swatch.id}
                    aria-label={`${option.name} (${option.english}), shade ${column + 1}, ${swatch.color}`}
                    title={`${option.name} · ${column + 1} · ${swatch.color}`}
                    aria-pressed={swatch.id === value}
                    tabindex={swatch.id === tabbableShade ? 0 : -1}
                    onfocus={() => {
                      activeShade = swatch.id
                    }}
                    onkeydown={event => navigate(event, row, column)}
                    onclick={() => {
                      onchange(swatch.id)
                      close()
                    }}
                  >
                    <span
                      class="size-2 rounded-full"
                      style:background-color={swatch.color}
                      aria-hidden="true"
                    ></span>
                  </button>
                {/each}
              </div>
            </div>
          {/if}
        {:else}
          <p class="px-3 py-2 text-sm text-rp-subtle">No matching inks.</p>
        {/each}
      </div>
    {/if}
  {/snippet}
  {#snippet footer(close)}
    <div class="flex w-full flex-col gap-2">
      {#if onreset}
        <button
          type="button"
          class="item-state item-state--menu cursor-pointer rounded-lg px-3 py-2 text-sm font-normal"
          aria-pressed={automatic}
          onclick={() => {
            onreset?.()
            close()
          }}>Automatic</button
        >
      {/if}
      <a
        class="link mx-auto inline-flex items-center gap-2 text-sm text-rp-subtle"
        href="https://madoromi.moonwit.ch/"
        target="_blank"
        rel="noreferrer"
      >
        <span>Inks by Madoromi</span><HugeiconsIcon
          icon={HeartIcon}
          size={16}
          strokeWidth={1.5}
          class="shrink-0 text-(--color-rose)"
          aria-hidden="true"
        />
      </a>
    </div>
  {/snippet}
</FilterPopover>

<style lang="postcss">
  @reference '../../app.css';
  :global(.btn.ink-picker-trigger--filled) {
    --size: 1.5rem;
    min-height: var(--size);
  }
  .ink-picker-options {
    max-height: min(55vh, 22rem);
    container-type: inline-size;
  }
  :global(.filter-popover.ink-picker-popover) {
    width: max-content;
    min-width: min(24rem, calc(100vw - 1.5rem));
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
  .ink-picker__measure-dots--shades {
    width: 14rem;
  }
  .ink-picker__shade-row {
    @apply grid items-center gap-3 px-3 py-2 text-sm font-normal;
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .ink-picker__shade {
    @apply flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-full;
  }
  @container (max-width: 26rem) {
    .ink-picker__shade-row {
      grid-template-columns: minmax(0, 1fr);
      gap: 0.25rem;
    }
  }
</style>
