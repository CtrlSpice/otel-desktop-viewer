<script lang="ts">
  import { normalizeChartAnnouncement } from '@/components/metrics/utils/chart-keyboard-cursor'

  type Props = {
    id: string
    label: string
    instructions: string
    readout: string
    shortcuts: readonly string[]
    onKeydown: (event: KeyboardEvent) => void
    onFocusChange?: (focused: boolean) => void
    roleDescription?: string
  }

  let {
    id,
    label,
    instructions,
    readout,
    shortcuts,
    onKeydown,
    onFocusChange,
    roleDescription = 'interactive chart',
  }: Props = $props()
  let focused = $state(false)
  let normalizedReadout = $derived(normalizeChartAnnouncement(readout))
  let keyShortcuts = $derived(shortcuts.join(' '))

  function setFocused(next: boolean) {
    focused = next
    onFocusChange?.(next)
  }
</script>

<!-- The application role is the intentional interaction contract for this
     single-focus custom chart widget; Svelte's static role table does not
     classify it as interactive. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  {id}
  class="chart-keyboard-surface"
  role="application"
  aria-roledescription={roleDescription}
  aria-label={label}
  aria-describedby={`${id}-instructions`}
  aria-keyshortcuts={keyShortcuts}
  tabindex="0"
  onfocus={() => setFocused(true)}
  onblur={() => setFocused(false)}
  onkeydown={onKeydown}
>
  <span id={`${id}-instructions`} class="sr-only">{instructions}</span>
  {#if normalizedReadout}
    <div class="chart-keyboard-readout" aria-hidden="true">
      {normalizedReadout}
    </div>
  {/if}
  <div class="sr-only" role="status" aria-live="polite" aria-atomic="true">
    {focused ? normalizedReadout : ''}
  </div>
</div>
