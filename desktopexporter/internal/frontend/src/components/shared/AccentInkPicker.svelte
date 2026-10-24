<script lang="ts">
  import InkPicker from './InkPicker.svelte'
  import { ACCENT_INKS, restoreAccentInk } from '@/themes/accent-inks'
  import { accentInk } from '@/state/accent-ink.svelte'

  const chosen = $derived(ACCENT_INKS.find(ink => ink.id === accentInk.value)!)
  const gradient = $derived(
    `linear-gradient(to right in oklab, ${chosen.swatches.map(swatch => swatch.color).join(', ')})`
  )
</script>

<InkPicker
  label={`Accent ink: ${chosen.name} (${chosen.english})`}
  triggerClass="btn btn-ghost btn-sm chart-header-control theme-ink tooltip tooltip-left"
  options={ACCENT_INKS}
  value={accentInk.value}
  onchange={id => accentInk.set(restoreAccentInk(id))}
>
  {#snippet triggerContent()}
    <span
      class="h-2 w-6 shrink-0 rounded-full"
      style:background-image={gradient}
      aria-hidden="true"
    ></span>
    <span>{chosen.name}</span>
  {/snippet}
</InkPicker>
