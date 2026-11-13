<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import IconButton from './IconButton.svelte'
  import {
    PanelLeftCloseIcon,
    PanelLeftOpenIcon,
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
    onclick: (event: MouseEvent) => void
  } = $props()

  const label = $derived(
    `${open ? 'Collapse' : 'Open'} ${side === 'left' ? 'sidebar' : 'details'}`
  )
</script>

<IconButton {label} aria-expanded={open} aria-controls={controls} {onclick}>
  <span class="inline-flex" class:panel-toggle__mirror={side === 'right'}>
    <HugeiconsIcon
      icon={PanelLeftOpenIcon}
      altIcon={PanelLeftCloseIcon}
      showAlt={!open}
      size={17}
      strokeWidth={1.5}
      aria-hidden="true"
    />
  </span>
</IconButton>

<style>
  .panel-toggle__mirror {
    transform: scaleX(-1);
  }
</style>
