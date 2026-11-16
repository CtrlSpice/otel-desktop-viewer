// @vitest-environment jsdom
import { render } from '@testing-library/svelte'
import { tick } from 'svelte'
import { describe, expect, it } from 'vitest'
import ChartKeyboardStateHarness from '@/test/ChartKeyboardStateHarness.svelte'
import {
  lineCursorAt,
  type KeyboardLine,
  type LineCursor,
  type LineCursorCommand,
  type OrderedCursor,
  type OrderedCursorCommand,
} from './chart-keyboard-cursor'
import type { ChartKeyboardCursorState } from './chart-keyboard-state.svelte'

describe('chart keyboard external selection identity', () => {
  it('distinguishes signed zero in one-key identities', async () => {
    let state:
      ChartKeyboardCursorState<OrderedCursor, OrderedCursorCommand> | undefined
    const view = render(ChartKeyboardStateHarness, {
      keys: [0, -0, 'away'],
      selectedKey: 0,
      onordered: next => {
        state = next
      },
    })

    state!.setFocused(true)
    state!.move('last-item')
    await view.rerender({ selectedKey: -0 })
    await tick()

    expect(Object.is(state!.current?.key, -0)).toBe(true)
  })

  it('distinguishes delimiter-containing two-key identities', async () => {
    const lines: KeyboardLine[] = [
      {
        key: 'alpha',
        points: [{ key: 'beta\u0000string:gamma', timestampMs: 1 }],
      },
      {
        key: 'alpha\u0000string:beta',
        points: [{ key: 'gamma', timestampMs: 2 }],
      },
    ]
    const first = lineCursorAt(lines, { lineKey: lines[0].key, timestampMs: 1 })
    const second = lineCursorAt(lines, {
      lineKey: lines[1].key,
      timestampMs: 2,
    })
    let state:
      ChartKeyboardCursorState<LineCursor, LineCursorCommand> | undefined
    const view = render(ChartKeyboardStateHarness, {
      lines,
      selectedLineCursor: first,
      online: next => {
        state = next
      },
    })

    state!.setFocused(true)
    await view.rerender({ selectedLineCursor: second })
    await tick()

    expect(state!.current).toMatchObject({
      lineKey: 'alpha\u0000string:beta',
      pointKey: 'gamma',
    })
  })

  it('records a null transition before the same one-key selection returns', async () => {
    let state:
      ChartKeyboardCursorState<OrderedCursor, OrderedCursorCommand> | undefined
    const view = render(ChartKeyboardStateHarness, {
      keys: ['first', 'second'],
      selectedKey: 'first',
      onordered: next => {
        state = next
      },
    })

    state!.setFocused(true)
    state!.move('last-item')
    await view.rerender({ selectedKey: null })
    await tick()
    expect(state!.current?.key).toBe('second')

    await view.rerender({ selectedKey: 'first' })
    await tick()
    expect(state!.current?.key).toBe('first')
  })
})
