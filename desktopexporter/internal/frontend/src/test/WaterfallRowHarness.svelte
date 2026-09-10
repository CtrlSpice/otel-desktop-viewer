<script lang="ts">
  // Production virtualizes this explicit ARIA row beneath transparent div
  // wrappers rather than a native table. Mirror that accessibility ancestry.
  import type { ComponentProps } from 'svelte'
  import WaterfallRow from '@/components/traces/Waterfall/WaterfallRow.svelte'

  let props: ComponentProps<typeof WaterfallRow> = $props()
</script>

<div role="treegrid" aria-label="Test waterfall">
  <div>
    <WaterfallRow {...props} />
  </div>
</div>
