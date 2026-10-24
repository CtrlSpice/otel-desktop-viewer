<script lang="ts">
  import InkPicker from './InkPicker.svelte'
  import GraphInkSlider from './GraphInkSlider.svelte'
  import {
    GRAPH_INKS,
    GRAPH_SWATCHES,
    INK_SLIDER_THRESHOLD,
    graphSwatch,
  } from '@/themes/graph-inks'
  import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'
  import { graphColorAvailability } from '@/utils/graph-color-availability'

  let {
    scope,
    identity,
    label,
    color,
    compactTrigger = false,
  }: {
    scope: string
    identity: string
    label: string
    color?: string
    compactTrigger?: boolean
  } = $props()
  const override = $derived(graphColorOverrides.get(scope, identity))
  const availability = $derived(graphColorAvailability(scope, identity))
  let useSliders = $state(false)
  const value = $derived(
    override?.id ?? GRAPH_SWATCHES.find(swatch => swatch.color === color)?.id
  )

  function choose(id: string) {
    const swatch = graphSwatch(id)
    if (
      swatch &&
      !graphColorAvailability(scope, identity).unavailable.has(swatch.color)
    ) {
      graphColorOverrides.set(scope, identity, swatch.id)
    }
  }
</script>

{#snippet sliders(
  option: { id: string; name: string; english: string },
  close: () => void
)}
  <GraphInkSlider
    ink={option.id}
    label={`${option.name} (${option.english})`}
    {value}
    unavailable={availability.unavailable}
    onchange={choose}
    {close}
  />
{/snippet}

<InkPicker
  {label}
  options={GRAPH_INKS}
  {value}
  triggerColor={color}
  {compactTrigger}
  mode="shade"
  automatic={!override}
  unavailable={availability.unavailable}
  shadeControls={useSliders ? sliders : undefined}
  onOpenChange={open => {
    // Decide once per opening so crossing the threshold cannot replace a thumb
    // during a drag. Saved curve colours remain editable even after resets.
    if (open)
      useSliders =
        availability.freePresets < INK_SLIDER_THRESHOLD ||
        Boolean(override && !override.shade)
  }}
  onchange={choose}
  onreset={() => graphColorOverrides.set(scope, identity, null)}
/>
