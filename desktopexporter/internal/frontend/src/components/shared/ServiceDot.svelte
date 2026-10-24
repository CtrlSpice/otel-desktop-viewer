<script lang="ts">
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'

  let { service }: { service: string } = $props()
</script>

<span
  class="inline-block size-2 shrink-0 rounded-full"
  style:background-color={traceServiceColor(service, themeSignal.value)}
  aria-hidden="true"
></span>
