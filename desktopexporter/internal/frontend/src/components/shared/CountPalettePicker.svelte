<script lang="ts">
  import InkPicker from './InkPicker.svelte'
  import { COUNT_PALETTES, restoreCountPalette } from '@/themes/count-palettes'
  import { countSwatches } from '@/utils/chart-palette'
  import { countPalette } from '@/state/count-palette.svelte'
  import { themeSignal } from '@/state/theme.svelte'

  let { label = 'Count palette' }: { label?: string } = $props()
  const options = $derived(
    COUNT_PALETTES.map(ink => ({
      ...ink,
      swatches: countSwatches(7, themeSignal.value, ink.id).map(
        (color, index) => ({ id: `${ink.id}:${index}`, color })
      ),
    }))
  )
</script>

<InkPicker
  {label}
  {options}
  value={countPalette.value}
  onchange={id => countPalette.set(restoreCountPalette(id))}
/>
