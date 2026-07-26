<script lang="ts">
  import { getRouteContext } from '@/contexts/route-context.svelte'

  // createRouteContext() runs $effect, so it only works inside a component.
  // This probe renders whatever the context currently reports so tests can
  // observe route changes through the DOM.
  const routeContext = getRouteContext()
</script>

<output data-testid="route-path">{routeContext.route.path}</output>
<output data-testid="route-query">
  {JSON.stringify(routeContext.route.query)}
</output>
