<script lang="ts">
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    FallingStarIcon,
    Moon02Icon,
    Sun03Icon,
  } from '@hugeicons/core-free-icons'
  import { createPopoverID, setupAnchorPopover } from './utils/anchor-popover'
  import chartAccents from '@/themes/chart-accents.json'

  const themes = [
    {
      id: 'rose-pine-dawn',
      name: 'Dawn',
      mode: 'Light',
      icon: Sun03Icon,
      accent: '#907aa9',
    },
    {
      id: 'rose-pine-moon',
      name: 'Moon',
      mode: 'Dark',
      icon: Moon02Icon,
      accent: '#c4a7e7',
    },
    {
      id: 'rose-pine',
      name: 'Pine',
      mode: 'Dark',
      icon: FallingStarIcon,
      accent: '#c4a7e7',
    },
  ] as const
  type Theme = (typeof themes)[number]

  const popoverID = createPopoverID('appearance')
  let triggerEl = $state<HTMLButtonElement | null>(null)
  let popoverEl = $state<HTMLDivElement | null>(null)
  let open = $state(false)
  let selected = $state<Theme['id']>('rose-pine-moon')
  const current = $derived(themes.find(theme => theme.id === selected)!)
  const label = $derived(`${current.name} ${current.mode}`)

  function apply(theme: Theme) {
    selected = theme.id
    document.documentElement.dataset.theme = theme.id
    document.documentElement.removeAttribute('data-palette')
    localStorage.setItem('theme', theme.id)
    localStorage.removeItem('theme-palette')
  }

  onMount(() => {
    const theme = localStorage.getItem('theme')
    const original = themes.find(option => option.id === theme)
    if (original) {
      apply(original)
      return
    }
    const light =
      theme === 'madoromi-day'
        ? true
        : theme === 'madoromi-night'
          ? false
          : !window.matchMedia('(prefers-color-scheme: dark)').matches
    apply(themes[light ? 0 : 1])
  })

  $effect(() => {
    if (!triggerEl || !popoverEl) return
    return setupAnchorPopover({
      trigger: triggerEl,
      popover: popoverEl,
      anchor: 'below-end',
      onOpenChange: next => {
        open = next
      },
    })
  })

  function choose(theme: Theme) {
    apply(theme)
    popoverEl?.hidePopover()
    triggerEl?.focus()
  }
</script>

<button
  bind:this={triggerEl}
  type="button"
  popovertarget={popoverID}
  aria-expanded={open}
  aria-label={`Appearance: ${label}`}
  data-tip={`Appearance: ${label}`}
  class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content tooltip tooltip-bottom"
>
  <span class="appearance-picker__icon">
    {#key label}
      <span class="appearance-picker__fade" transition:fade={{ duration: 150 }}>
        <HugeiconsIcon
          icon={current.icon}
          size={17}
          strokeWidth={1.5}
          aria-hidden="true"
        />
      </span>
    {/key}
  </span>
</button>
<div
  bind:this={popoverEl}
  id={popoverID}
  popover="auto"
  class="anchor-popover anchor-popover--anchored w-72 p-2"
>
  <fieldset>
    <legend class="px-2 py-1 text-xs text-rp-subtle">Appearance</legend>
    {#each themes as theme}
      <label class="appearance-picker__choice">
        <span style:color={theme.accent}>
          <HugeiconsIcon
            icon={theme.icon}
            size={18}
            strokeWidth={1.5}
            aria-hidden="true"
          />
        </span>
        <span class="appearance-picker__name">{theme.name}</span>
        <span class="text-xs text-rp-subtle">{theme.mode}</span>
        <input
          type="radio"
          name={popoverID}
          class="radio radio-xs"
          aria-label={`${theme.name} ${theme.mode}`}
          checked={selected === theme.id}
          onchange={() => choose(theme)}
        />
        <span class="appearance-picker__swatches" aria-hidden="true">
          {#each [theme.accent, ...Object.values(chartAccents[theme.id])] as color}
            <span class="appearance-picker__swatch" style:--swatch={color}
            ></span>
          {/each}
        </span>
      </label>
    {/each}
  </fieldset>
</div>

<style lang="postcss">
  @reference "../../app.css";

  .appearance-picker__choice {
    @apply grid cursor-pointer items-center gap-x-3 gap-y-2 rounded-md px-3 py-3;
    grid-template-columns: auto 1fr auto auto;
  }

  .appearance-picker__name {
    @apply text-sm;
  }

  .appearance-picker__choice:hover .appearance-picker__name {
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .appearance-picker__swatches {
    grid-column: 2 / -1;
    display: flex;
    gap: 5px;
  }

  .appearance-picker__swatch {
    width: 19px;
    height: 7px;
    border-radius: 999px;
    background: var(--swatch);
  }

  .appearance-picker__icon,
  .appearance-picker__fade {
    display: inline-grid;
    place-items: center;
  }

  .appearance-picker__fade {
    grid-area: 1 / 1;
  }
</style>
