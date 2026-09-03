<script lang="ts">
  import WaterfallView from '@/components/traces/Waterfall/WaterfallView.svelte'
  import { createRouteContext } from '@/contexts/route-context.svelte'
  import { createTimeContext } from '@/contexts/time-context.svelte'
  import type { TraceData } from '@/types/api-types'

  type Props = {
    trace: TraceData
  }

  let { trace }: Props = $props()

  createRouteContext()
  createTimeContext()

  const ignoreSelection: (spanID: string) => void = () => undefined
</script>

<div class="arm-a-waterfall-host">
  <WaterfallView
    spans={trace.spans}
    selectedSpanID={null}
    onSelectSpan={ignoreSelection}
    loading={false}
  />
</div>

<style>
  .arm-a-waterfall-host {
    box-sizing: border-box;
    width: 100vw;
    height: 100vh;
    min-width: 640px;
    min-height: 480px;
    overflow: hidden;
  }
</style>
