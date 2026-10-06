<script lang="ts">
  import type { Component } from 'svelte'
  import { createRouteContext } from '@/contexts/route-context.svelte'
  import { createTimeContext } from '@/contexts/time-context.svelte'

  // testing-library reserves the top-level `props` key during rerender.
  type Props = {
    component: Component<any>
    componentProps?: Record<string, unknown>
  }
  let { component: TestComponent, componentProps = {} }: Props = $props()

  createRouteContext()
  createTimeContext()
</script>

<TestComponent {...componentProps} />
