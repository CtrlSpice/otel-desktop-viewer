<script lang="ts">
  import {
    getTimeContext,
    type TimeContext,
  } from '@/contexts/time-context.svelte'

  // createTimeContext() runs $effect, so it only works inside a component.
  // This probe renders whatever the context currently reports so tests can
  // observe selection/tz changes through the DOM, and also hands the live
  // context object back to the test via a callback prop so tests can drive
  // setSelection/setTz directly without encoding args through the DOM.
  type Props = {
    onContext?: (context: TimeContext) => void
  }
  let { onContext }: Props = $props()

  const timeContext = getTimeContext()
  $effect(() => {
    onContext?.(timeContext)
  })
</script>

<output data-testid="selection-type">{timeContext.selection.type}</output>
<output data-testid="selection-start">
  {'start' in timeContext.selection ? timeContext.selection.start : ''}
</output>
<output data-testid="selection-end">
  {'end' in timeContext.selection ? timeContext.selection.end : ''}
</output>
<output data-testid="selection-preset-index">
  {timeContext.selection.type === 'preset'
    ? timeContext.selection.presetIndex
    : ''}
</output>
<output data-testid="selection-duration">
  {timeContext.selection.type === 'preset'
    ? timeContext.selection.durationMs
    : ''}
</output>
<output data-testid="tz">{timeContext.tz}</output>
