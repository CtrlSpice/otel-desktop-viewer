<script lang="ts">
  import type { Component } from 'svelte'
  import { createRouteContext } from '@/contexts/route-context.svelte'
  import { createTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    component: Component<any>
    props?: Record<string, unknown>
  }
  let { component: TestComponent, props = {} }: Props = $props()

  createRouteContext()
  createTimeContext()
</script>

<TestComponent {...props} />
