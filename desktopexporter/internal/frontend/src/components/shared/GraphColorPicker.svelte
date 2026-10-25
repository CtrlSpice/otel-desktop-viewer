<script lang="ts">
  import 'vanilla-colorful/hex-color-picker.js'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { RefreshIcon } from '@hugeicons/core-free-icons'
  import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'
  import { graphInkFamily } from '@/utils/graph-ink-family'
  import FilterPopover from './Toolbar/FilterPopover.svelte'

  let {
    scope,
    identity,
    label,
    color,
    compact = false,
  }: {
    scope: string
    identity: string
    label: string
    color: string
    compact?: boolean
  } = $props()

  const override = $derived(graphColorOverrides.get(scope, identity))
  const selected = $derived(override?.color ?? color)
  const family = $derived(graphInkFamily(selected, override?.id))
  let hex = $state('')
  let input = $state<HTMLInputElement>()
  $effect(() => {
    hex = selected
  })

  function choose(value: string) {
    graphColorOverrides.set(scope, identity, value)
  }
</script>

<FilterPopover
  {label}
  compact
  role="dialog"
  showHeader={false}
  popoverClass="graph-colour-popover"
  triggerClass={`series-colour-edit header-icon-button tooltip tooltip-left ${compact ? 'series-colour-edit--compact' : ''}`}
  onOpenChange={open => {
    if (open) {
      hex = selected
      input?.focus({ preventScroll: true })
    }
  }}
>
  {#snippet icon()}
    <span
      class="size-2 shrink-0 rounded-full"
      style:background-color={selected}
      aria-hidden="true"
    ></span>
  {/snippet}
  {#snippet children()}
    <div class="graph-colour-picker">
      <hex-color-picker
        color={selected}
        oncolor-changed={(event: CustomEvent<{ value: string }>) =>
          choose(event.detail.value)}
      ></hex-color-picker>
      <div class="flex items-center gap-2">
        <label
          class="flex min-w-0 flex-1 items-center gap-2 text-sm text-subtle"
        >
          Hex
          <input
            bind:this={input}
            class="input input-sm min-w-0 w-full"
            aria-label="Colour value"
            type="text"
            spellcheck={false}
            maxlength={7}
            pattern="#[0-9a-fA-F]{6}"
            bind:value={hex}
            oninput={event => {
              const value = event.currentTarget.value
              if (/^#[0-9a-fA-F]{6}$/.test(value)) choose(value)
            }}
            onblur={() => {
              hex = selected
            }}
          />
        </label>
        <button
          type="button"
          class="header-icon-button tooltip tooltip-left"
          aria-label={`Reset ${label.toLowerCase()} to automatic`}
          data-tip="Reset to automatic colour"
          disabled={!override}
          onclick={() => {
            graphColorOverrides.set(scope, identity, null)
            input?.focus({ preventScroll: true })
          }}
        >
          <HugeiconsIcon
            icon={RefreshIcon}
            size={16}
            strokeWidth={1.5}
            aria-hidden="true"
          />
        </button>
      </div>
      <span class="text-sm text-subtle" data-ink-family
        >{family.exact ? '' : 'Closest ink · '}{family.ink.name}</span
      >
      <div
        class="graph-colour-picker__swatches"
        role="group"
        aria-label={`${family.ink.name} ink shades`}
      >
        {#each family.ink.swatches as swatch (swatch.id)}
          <button
            type="button"
            class="header-icon-button tooltip tooltip-top"
            aria-label={`${family.ink.name} shade ${swatch.shade}`}
            data-tip={`${family.ink.name} · ${swatch.shade}`}
            aria-pressed={selected.toLowerCase() === swatch.color}
            onclick={() => choose(swatch.id)}
          >
            <span
              class="size-2 shrink-0 rounded-full"
              style:background-color={swatch.color}
              aria-hidden="true"
            ></span>
          </button>
        {/each}
      </div>
    </div>
  {/snippet}
</FilterPopover>

<style lang="postcss">
  @reference '../../app.css';
  :global(.anchor-popover.graph-colour-popover) {
    min-width: 0;
    width: max-content;
  }
  .graph-colour-picker {
    @apply flex flex-col gap-2;
    width: min(16rem, calc(100vw - 3rem));
  }
  .graph-colour-picker__swatches {
    @apply flex justify-between;
  }
  hex-color-picker {
    width: 100%;
    height: 11rem;
  }
  hex-color-picker::part(saturation) {
    bottom: 24px;
    border-radius: var(--radius-lg);
  }
  hex-color-picker::part(hue) {
    height: 12px;
    border-radius: var(--radius-full);
  }
  hex-color-picker::part(saturation-pointer),
  hex-color-picker::part(hue-pointer) {
    width: 16px;
    height: 16px;
  }
  :global(.series-colour-edit--compact) {
    width: 24px;
    height: 24px;
  }
</style>
