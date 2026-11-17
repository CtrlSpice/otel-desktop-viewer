<script lang="ts">
  import { cost } from './model'
  let { average, max }: { average: number | null; max: number } = $props()
</script>

<div class="metric cost">
  <span class="bar-track" aria-hidden="true"
    ><span style="width:{((average ?? 0) / max) * 100}%"></span></span
  >
  <strong>{cost(average, 'complete')}</strong>
</div>
