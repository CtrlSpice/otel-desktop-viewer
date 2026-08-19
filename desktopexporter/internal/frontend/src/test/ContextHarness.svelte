<script lang="ts">
  import type { Component } from 'svelte'
  import { createRouteContext } from '@/contexts/route-context.svelte'
  import { createTimeContext } from '@/contexts/time-context.svelte'

  // Named componentProps, not props: testing-library's rerender treats a
  // top-level `props` key in its argument as the deprecated calling form and
  // unwraps it, so a harness prop by that name makes every rerender silently
  // update the wrong layer -- assertions after it pass against the old props.
  type Props = {
    component: Component<any>
    componentProps?: Record<string, unknown>
  }
  let { component: TestComponent, componentProps = {} }: Props = $props()

  createRouteContext()
  createTimeContext()
</script>

<TestComponent {...componentProps} />
