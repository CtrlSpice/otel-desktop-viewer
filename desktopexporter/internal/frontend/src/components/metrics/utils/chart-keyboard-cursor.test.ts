import { describe, expect, it } from 'vitest'
import {
  gridCursorAt,
  gridCursorCommandForKey,
  lineCursorAt,
  lineCursorCommandForKey,
  moveGridCursor,
  moveLineCursor,
  moveOrderedCursor,
  normalizeChartAnnouncement,
  orderedCursorCommandForKey,
  reconcileGridCursor,
  reconcileLineCursor,
  reconcileOrderedCursor,
  type KeyboardLine,
} from './chart-keyboard-cursor'

const lines: KeyboardLine[] = [
  { key: 'alpha', points: keyboardPoints([10, 20, 30]) },
  { key: 'beta', points: keyboardPoints([12, 27]) },
  { key: 'empty', points: [] },
  { key: 'gamma', points: keyboardPoints([5, 40]) },
]

function keyboardPoints(timestamps: number[]) {
  return timestamps.map((timestampMs, index) => ({
    key: `${timestampMs}:${index}`,
    timestampMs,
  }))
}

describe('line chart keyboard cursor', () => {
  it('moves between points and clamps at line boundaries without wrapping', () => {
    const start = lineCursorAt(lines, { lineKey: 'alpha', timestampMs: 20 })
    const previous = moveLineCursor(lines, start, 'previous-point')
    const clamped = moveLineCursor(lines, previous, 'previous-point')

    expect(previous).toMatchObject({ lineKey: 'alpha', timestampMs: 10 })
    expect(clamped).toBe(previous)
    expect(moveLineCursor(lines, start, 'line-end')).toMatchObject({
      lineKey: 'alpha',
      timestampMs: 30,
    })
  })

  it('moves between non-empty lines at the nearest timestamp', () => {
    const start = lineCursorAt(lines, { lineKey: 'alpha', timestampMs: 30 })
    const next = moveLineCursor(lines, start, 'next-line')
    const afterEmpty = moveLineCursor(lines, next, 'next-line')

    expect(next).toMatchObject({ lineKey: 'beta', timestampMs: 27 })
    expect(afterEmpty).toMatchObject({ lineKey: 'gamma', timestampMs: 40 })
  })

  it('uses modified Home and End for the global time extremes', () => {
    const start = lineCursorAt(lines, { lineKey: 'beta', timestampMs: 12 })

    expect(moveLineCursor(lines, start, 'global-start')).toMatchObject({
      lineKey: 'gamma',
      timestampMs: 5,
    })
    expect(moveLineCursor(lines, start, 'global-end')).toMatchObject({
      lineKey: 'gamma',
      timestampMs: 40,
    })
    expect(lineCursorCommandForKey({ key: 'Home', metaKey: true })).toBe(
      'global-start'
    )
    expect(lineCursorCommandForKey({ key: 'End', ctrlKey: true })).toBe(
      'global-end'
    )
  })

  it('reconciles by stable line key and nearest timestamp', () => {
    const current = lineCursorAt(lines, { lineKey: 'beta', timestampMs: 27 })
    const refreshed: KeyboardLine[] = [
      { key: 'beta', points: keyboardPoints([13, 26, 39]) },
      { key: 'alpha', points: keyboardPoints([11]) },
    ]

    expect(reconcileLineCursor(refreshed, current)).toMatchObject({
      lineKey: 'beta',
      timestampMs: 26,
      lineIndex: 0,
      pointIndex: 1,
    })
  })

  it('preserves distinct exact identities at one millisecond coordinate', () => {
    const duplicateMillisecond: KeyboardLine[] = [
      {
        key: 'alpha',
        points: [
          { key: '100ns', timestampMs: 10, timestampNs: 10_000_100n },
          { key: '900ns', timestampMs: 10, timestampNs: 10_000_900n },
        ],
      },
    ]
    const second = lineCursorAt(duplicateMillisecond, {
      lineKey: 'alpha',
      timestampNs: 10_000_900n,
    })

    expect(second).toMatchObject({ pointKey: '900ns', pointIndex: 1 })
    expect(
      moveLineCursor(duplicateMillisecond, second, 'previous-point')
    ).toMatchObject({ pointKey: '100ns', pointIndex: 0 })
    expect(
      moveLineCursor(duplicateMillisecond, second, 'global-start')
    ).toMatchObject({ pointKey: '100ns', pointIndex: 0 })
    expect(
      moveLineCursor(
        duplicateMillisecond,
        lineCursorAt(duplicateMillisecond, {
          lineKey: 'alpha',
          timestampNs: 10_000_100n,
        }),
        'global-end'
      )
    ).toMatchObject({ pointKey: '900ns', pointIndex: 1 })
    expect(
      lineCursorAt(duplicateMillisecond, { timestampNs: 10_000_800n })
    ).toMatchObject({ pointKey: '900ns', pointIndex: 1 })
    expect(
      lineCursorAt(duplicateMillisecond, {
        lineKey: 'alpha',
        timestampNs: 10_000_500n,
        timestampMs: 10,
        requireExact: true,
      })
    ).toBeNull()
  })
})

describe('ordered chart keyboard cursor', () => {
  it('preserves a stable key across reordering and clamps movement', () => {
    const current = reconcileOrderedCursor(['low', 'mid', 'high'], {
      key: 'mid',
      index: 1,
    })
    const reordered = reconcileOrderedCursor(['mid', 'high'], current)

    expect(reordered).toEqual({ key: 'mid', index: 0 })
    expect(moveOrderedCursor(['mid', 'high'], reordered, 'previous-item')).toBe(
      reordered
    )
    expect(moveOrderedCursor(['mid', 'high'], reordered, 'last-item')).toEqual({
      key: 'high',
      index: 1,
    })
  })

  it('ignores modified boundaries that a histogram does not declare', () => {
    expect(
      orderedCursorCommandForKey({ key: 'Home', metaKey: true })
    ).toBeNull()
  })
})

describe('grid chart keyboard cursor', () => {
  it('navigates a rectangular grid without wrapping', () => {
    const columns = [100, 200, 300]
    const rows = ['high', 'low']
    const start = gridCursorAt(columns, rows, 200, 'high')
    const down = moveGridCursor(columns, rows, start, 'next-row')
    const clamped = moveGridCursor(columns, rows, down, 'next-row')

    expect(down).toEqual({
      columnKey: 200,
      rowKey: 'low',
      columnIndex: 1,
      rowIndex: 1,
    })
    expect(clamped).toBe(down)
    expect(moveGridCursor(columns, rows, down, 'row-start')).toMatchObject({
      columnKey: 100,
      rowKey: 'low',
    })
  })

  it('supports modified global boundaries and stable reconciliation', () => {
    const current = gridCursorAt([100, 200], ['high', 'low'], 200, 'low')
    expect(gridCursorCommandForKey({ key: 'Home', ctrlKey: true })).toBe(
      'grid-start'
    )
    expect(
      moveGridCursor([100, 200], ['high', 'low'], current, 'grid-start')
    ).toEqual({
      columnKey: 100,
      rowKey: 'high',
      columnIndex: 0,
      rowIndex: 0,
    })
    expect(reconcileGridCursor([200, 300], ['low', 'zero'], current)).toEqual({
      columnKey: 200,
      rowKey: 'low',
      columnIndex: 0,
      rowIndex: 0,
    })
  })
})

it('normalizes chart announcements for matching visual and live text', () => {
  expect(normalizeChartAnnouncement('  Line 1  of 2 .\n Value  10 ms  ')).toBe(
    'Line 1 of 2. Value 10 ms'
  )
})
