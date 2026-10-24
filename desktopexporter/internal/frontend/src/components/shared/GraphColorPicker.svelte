<script lang="ts">
  import InkPicker from './InkPicker.svelte'
  import { GRAPH_INKS, GRAPH_SWATCHES, graphSwatch } from '@/themes/graph-inks'
  import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'

  let {
    scope,
    identity,
    label,
    color,
    filled = false,
  }: {
    scope: string
    identity: string
    label: string
    color?: string
    filled?: boolean
  } = $props()
  const override = $derived(graphColorOverrides.get(scope, identity))
  const value = $derived(
    override?.id ?? GRAPH_SWATCHES.find(swatch => swatch.color === color)?.id
  )
</script>

<InkPicker
  {label}
  options={GRAPH_INKS}
  {value}
  triggerColor={filled ? color : undefined}
  mode="shade"
  automatic={!override}
  onchange={id => {
    const swatch = graphSwatch(id)
    if (swatch) graphColorOverrides.set(scope, identity, swatch.id)
  }}
  onreset={() => graphColorOverrides.set(scope, identity, null)}
/>
