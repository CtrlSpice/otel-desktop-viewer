<script lang="ts">
  import { untrack } from 'svelte'
  import {
    createLineChartKeyboardCursor,
    createOrderedChartKeyboardCursor,
    type ChartKeyboardCursorState,
  } from '@/features/metrics/Charts/chart-keyboard-state.svelte'
  import type {
    CursorKey,
    KeyboardLine,
    LineCursor,
    LineCursorCommand,
    OrderedCursor,
    OrderedCursorCommand,
  } from '@/features/metrics/Charts/chart-keyboard-cursor'

  type Props = {
    keys?: readonly CursorKey[]
    lines?: readonly KeyboardLine[]
    selectedKey?: CursorKey | null
    selectedLineCursor?: LineCursor | null
    onordered?: (
      state: ChartKeyboardCursorState<OrderedCursor, OrderedCursorCommand>
    ) => void
    online?: (
      state: ChartKeyboardCursorState<LineCursor, LineCursorCommand>
    ) => void
  }

  let {
    keys = [],
    lines = [],
    selectedKey = null,
    selectedLineCursor = null,
    onordered,
    online,
  }: Props = $props()

  const ordered = createOrderedChartKeyboardCursor(
    () => keys,
    () => selectedKey
  )
  const line = createLineChartKeyboardCursor(
    () => lines,
    () => selectedLineCursor
  )

  untrack(() => {
    onordered?.(ordered)
    online?.(line)
  })
</script>
