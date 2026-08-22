<script lang="ts">
  // WaterfallRow renders a <tr>; mounting it directly (outside a <table>)
  // risks it rendering under a different insertion mode than production
  // markup ever exercises. This harness gives it the same table/tbody
  // ancestor WaterfallView provides.
  import type { ComponentProps } from 'svelte'
  import WaterfallRow from '@/components/traces/Waterfall/WaterfallRow.svelte'

  let props: ComponentProps<typeof WaterfallRow> = $props()
</script>

<table>
  <tbody>
    <WaterfallRow {...props} />
  </tbody>
</table>
