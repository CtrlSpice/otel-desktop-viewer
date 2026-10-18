<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    PanelLeftCloseIcon,
    PanelLeftOpenIcon,
    PanelRightCloseIcon,
    PanelRightOpenIcon,
  } from '@hugeicons/core-free-icons'

  let {
    side,
    open,
    controls,
    onclick,
  }: {
    side: 'left' | 'right'
    open: boolean
    controls: string
    onclick: () => void
  } = $props()

  const label = $derived(
    `${open ? 'Collapse' : 'Open'} ${side === 'left' ? 'sidebar' : 'details'}`
  )
  const icon = $derived(
    side === 'left'
      ? open
        ? PanelLeftCloseIcon
        : PanelLeftOpenIcon
      : open
        ? PanelRightCloseIcon
        : PanelRightOpenIcon
  )
</script>

<button
  type="button"
  class="panel-toggle btn btn-circle {open
    ? 'btn-soft btn-primary'
    : 'btn-ghost'}"
  aria-label={label}
  title={label}
  aria-expanded={open}
  aria-controls={controls}
  {onclick}
>
  <HugeiconsIcon {icon} size={16} strokeWidth={1.5} aria-hidden="true" />
</button>

<style lang="postcss">
  @reference "../../app.css";

  .panel-toggle {
    @apply size-10 shrink-0 border-0 p-0 shadow-none;
  }

  .panel-toggle[aria-expanded='false'] {
    @apply text-base-content/50 hover:text-base-content;
  }
</style>
