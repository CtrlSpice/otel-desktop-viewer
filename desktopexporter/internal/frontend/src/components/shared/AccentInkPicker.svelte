<script lang="ts">
  import InkPicker from './InkPicker.svelte'
  import {
    ACCENT_INKS,
    accentInkPreview,
    restoreAccentInk,
  } from '@/themes/accent-inks'
  import { accentInk } from '@/state/accent-ink.svelte'
  import { themeSignal } from '@/state/theme.svelte'
  import { getTheme } from '@/themes'

  const chosen = $derived(ACCENT_INKS.find(ink => ink.id === accentInk.value)!)
  const options = $derived(
    ACCENT_INKS.map(ink => ({
      ...ink,
      swatches: accentInkPreview(ink.id, getTheme(themeSignal.value).id).map(
        (color, index) => ({ id: `${ink.id}:${index}`, color })
      ),
    }))
  )
</script>

<InkPicker
  label={`Accent ink: ${chosen.name} (${chosen.english})`}
  triggerText="Ink"
  triggerClass="btn btn-ghost btn-sm chart-header-control theme-ink tooltip tooltip-left"
  {options}
  value={accentInk.value}
  onchange={id => accentInk.set(restoreAccentInk(id))}
/>
