<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { PencilEdit02Icon, RefreshIcon } from '@hugeicons/core-free-icons'
  import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'

  let {
    scope,
    identity,
    label,
    color,
  }: {
    scope: string
    identity: string
    label: string
    color: string
  } = $props()

  const override = $derived(graphColorOverrides.get(scope, identity))
  const selected = $derived(override?.color ?? color)
</script>

<div class="flex shrink-0 items-center">
  <label
    class="series-colour-edit header-icon-button tooltip tooltip-left"
    data-tip={label}
  >
    <HugeiconsIcon
      icon={PencilEdit02Icon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />
    <input
      type="color"
      class="series-colour-input"
      aria-label={label}
      value={selected}
      oninput={event =>
        graphColorOverrides.set(scope, identity, event.currentTarget.value)}
    />
  </label>
  {#if override}
    <button
      type="button"
      class="header-icon-button tooltip tooltip-left"
      aria-label={`Reset ${label.toLowerCase()} to automatic`}
      data-tip="Reset to automatic colour"
      onclick={() => graphColorOverrides.set(scope, identity, null)}
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

<style lang="postcss">
  @reference '../../app.css';
  .series-colour-edit {
    position: relative;
  }
  .series-colour-input {
    @apply absolute inset-0 size-full cursor-pointer opacity-0;
  }
  .series-colour-edit:has(:focus-visible) {
    outline: 2px solid var(--color-primary);
    outline-offset: 2px;
  }
</style>
