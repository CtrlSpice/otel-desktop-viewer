<script lang="ts">
  import InkPicker from './InkPicker.svelte'
  import { ACCENT_INKS, restoreAccentInk } from '@/themes/accent-inks'
  import { accentInk } from '@/state/accent-ink.svelte'

  const chosen = $derived(ACCENT_INKS.find(ink => ink.id === accentInk.value)!)
</script>

<InkPicker
  label={`Accent ink: ${chosen.name} (${chosen.english})`}
  triggerText="Ink"
  triggerClass="btn btn-ghost btn-sm chart-header-control theme-ink tooltip tooltip-left"
  options={ACCENT_INKS}
  value={accentInk.value}
  onchange={id => accentInk.set(restoreAccentInk(id))}
/>
