<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
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
  const icon = $derived(open ? PanelLeftCloseIcon : PanelLeftOpenIcon)
</script>

<button
  type="button"
  class="panel-toggle btn btn-circle btn-ghost btn-sm shrink-0 text-sm tooltip"
  class:tooltip-right={side === 'left'}
  class:tooltip-left={side === 'right'}
  aria-label={label}
  data-tip={label}
  aria-expanded={open}
  aria-controls={controls}
  {onclick}
>
  <span class="inline-flex" class:panel-toggle__mirror={side === 'right'}>
    <HugeiconsIcon {icon} size={17} strokeWidth={1.5} aria-hidden="true" />
  </span>
</button>

<style lang="postcss">
  @reference "../../app.css";

  .panel-toggle {
    @apply text-base-content;
  }

  .panel-toggle[aria-expanded='true'] {
    @apply text-primary;
  }

  .panel-toggle__mirror {
    transform: scaleX(-1);
  }
</style>
