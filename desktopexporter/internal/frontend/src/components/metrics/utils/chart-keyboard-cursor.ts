export type CursorKey = string | number

export type KeyboardPoint = {
  key: CursorKey
  timestampMs: number
  timestampNs?: bigint
}

export function stablePointCursorKey(
  point: {
    sourceKey?: string
    timestampNs?: bigint
    timestampMs: number
  },
  fallbackIndex: number
): string {
  if (point.sourceKey) return `source:${point.sourceKey}`
  if (point.timestampNs !== undefined) return `timestamp:${point.timestampNs}`
  return `millisecond:${point.timestampMs}:index:${fallbackIndex}`
}

export type KeyboardLine = {
  key: string
  points: readonly KeyboardPoint[]
}

export type LineCursor = {
  lineKey: string
  pointKey: CursorKey
  timestampMs: number
  timestampNs?: bigint
  lineIndex: number
  pointIndex: number
}

export type LineCursorCommand =
  | 'previous-point'
  | 'next-point'
  | 'previous-line'
  | 'next-line'
  | 'line-start'
  | 'line-end'
  | 'global-start'
  | 'global-end'

export type OrderedCursor = {
  key: CursorKey
  index: number
}

export type OrderedCursorCommand =
  'previous-item' | 'next-item' | 'first-item' | 'last-item'

export type GridCursor = {
  columnKey: CursorKey
  rowKey: CursorKey
  columnIndex: number
  rowIndex: number
}

export type GridCursorCommand =
  | 'previous-column'
  | 'next-column'
  | 'previous-row'
  | 'next-row'
  | 'row-start'
  | 'row-end'
  | 'grid-start'
  | 'grid-end'

type KeyboardModifiers = {
  key: string
  altKey?: boolean
  ctrlKey?: boolean
  metaKey?: boolean
}

function clamp(value: number, max: number): number {
  return Math.max(0, Math.min(value, max))
}

function sameKey(a: CursorKey, b: CursorKey): boolean {
  return Object.is(a, b)
}

export function nearestTimestampIndex(
  timestamps: readonly number[],
  targetMs: number
): number {
  if (timestamps.length === 0) return -1

  let bestIndex = 0
  let bestDistance = Math.abs(timestamps[0]! - targetMs)
  for (let index = 1; index < timestamps.length; index++) {
    const distance = Math.abs(timestamps[index]! - targetMs)
    if (distance < bestDistance) {
      bestIndex = index
      bestDistance = distance
    }
  }
  return bestIndex
}

function pointTimestampNs(point: KeyboardPoint): bigint {
  return point.timestampNs ?? BigInt(Math.trunc(point.timestampMs)) * 1_000_000n
}

function nearestPointIndex(
  points: readonly KeyboardPoint[],
  targetMs: number,
  targetNs?: bigint
): number {
  if (targetNs === undefined) {
    return nearestTimestampIndex(
      points.map(point => point.timestampMs),
      targetMs
    )
  }
  if (points.length === 0) return -1

  let bestIndex = 0
  let bestDistance = pointTimestampNs(points[0]!) - targetNs
  if (bestDistance < 0n) bestDistance = -bestDistance
  for (let index = 1; index < points.length; index++) {
    let distance = pointTimestampNs(points[index]!) - targetNs
    if (distance < 0n) distance = -distance
    if (distance < bestDistance) {
      bestIndex = index
      bestDistance = distance
    }
  }
  return bestIndex
}

function inspectableLines(lines: readonly KeyboardLine[]) {
  return lines
    .map((line, lineIndex) => ({ line, lineIndex }))
    .filter(({ line }) => line.points.length > 0)
}

function makeLineCursor(
  current: LineCursor | null,
  line: KeyboardLine,
  lineIndex: number,
  pointIndex: number
): LineCursor {
  const point = line.points[pointIndex]!
  if (
    current?.lineKey === line.key &&
    sameKey(current.pointKey, point.key) &&
    current.timestampMs === point.timestampMs &&
    current.timestampNs === point.timestampNs &&
    current.lineIndex === lineIndex &&
    current.pointIndex === pointIndex
  ) {
    return current
  }
  return {
    lineKey: line.key,
    pointKey: point.key,
    timestampMs: point.timestampMs,
    timestampNs: point.timestampNs,
    lineIndex,
    pointIndex,
  }
}

export function lineCursorAt(
  lines: readonly KeyboardLine[],
  target: {
    lineKey?: string | null
    pointKey?: CursorKey | null
    timestampMs?: number | null
    timestampNs?: bigint | null
    requireExact?: boolean
  } = {}
): LineCursor | null {
  const available = inspectableLines(lines)
  if (available.length === 0) return null

  const requested =
    target.lineKey == null
      ? undefined
      : available.find(entry => entry.line.key === target.lineKey)
  if (target.requireExact && target.lineKey != null && !requested) return null
  const entry = requested ?? available[0]!
  const keyedIndex =
    target.pointKey == null
      ? -1
      : entry.line.points.findIndex(point =>
          sameKey(point.key, target.pointKey as CursorKey)
        )
  const timestampIndex =
    target.timestampNs == null
      ? -1
      : entry.line.points.findIndex(
          point => point.timestampNs === target.timestampNs
        )
  const pointIndex =
    keyedIndex >= 0
      ? keyedIndex
      : timestampIndex >= 0
        ? timestampIndex
        : target.timestampMs == null && target.timestampNs == null
          ? 0
          : nearestPointIndex(
              entry.line.points,
              target.timestampMs ?? Number(target.timestampNs! / 1_000_000n),
              target.timestampNs ?? undefined
            )
  if (
    target.requireExact &&
    (target.pointKey != null || target.timestampNs != null) &&
    keyedIndex < 0 &&
    timestampIndex < 0
  ) {
    return null
  }
  return makeLineCursor(
    null,
    entry.line,
    entry.lineIndex,
    Math.max(0, pointIndex)
  )
}

export function reconcileLineCursor(
  lines: readonly KeyboardLine[],
  current: LineCursor | null
): LineCursor | null {
  if (current === null) return lineCursorAt(lines)

  const available = inspectableLines(lines)
  if (available.length === 0) return null

  const keyed = available.find(entry => entry.line.key === current.lineKey)
  const entry =
    keyed ??
    available.reduce((best, candidate) =>
      Math.abs(candidate.lineIndex - current.lineIndex) <
      Math.abs(best.lineIndex - current.lineIndex)
        ? candidate
        : best
    )
  const keyedPointIndex = entry.line.points.findIndex(point =>
    sameKey(point.key, current.pointKey)
  )
  const timestampPointIndex =
    current.timestampNs === undefined
      ? -1
      : entry.line.points.findIndex(
          point => point.timestampNs === current.timestampNs
        )
  const pointIndex =
    keyedPointIndex >= 0
      ? keyedPointIndex
      : timestampPointIndex >= 0
        ? timestampPointIndex
        : nearestPointIndex(
            entry.line.points,
            current.timestampMs,
            current.timestampNs
          )
  return makeLineCursor(
    current,
    entry.line,
    entry.lineIndex,
    Math.max(0, pointIndex)
  )
}

export function moveLineCursor(
  lines: readonly KeyboardLine[],
  current: LineCursor | null,
  command: LineCursorCommand
): LineCursor | null {
  const cursor = reconcileLineCursor(lines, current)
  if (cursor === null) return null

  const available = inspectableLines(lines)
  const availableIndex = available.findIndex(
    entry => entry.lineIndex === cursor.lineIndex
  )
  if (availableIndex < 0) return cursor

  const currentEntry = available[availableIndex]!
  if (command === 'previous-point' || command === 'next-point') {
    const offset = command === 'previous-point' ? -1 : 1
    const pointIndex = clamp(
      cursor.pointIndex + offset,
      currentEntry.line.points.length - 1
    )
    return makeLineCursor(
      cursor,
      currentEntry.line,
      currentEntry.lineIndex,
      pointIndex
    )
  }

  if (command === 'previous-line' || command === 'next-line') {
    const offset = command === 'previous-line' ? -1 : 1
    const nextAvailableIndex = clamp(
      availableIndex + offset,
      available.length - 1
    )
    const nextEntry = available[nextAvailableIndex]!
    const pointIndex = nearestPointIndex(
      nextEntry.line.points,
      cursor.timestampMs,
      cursor.timestampNs
    )
    return makeLineCursor(
      cursor,
      nextEntry.line,
      nextEntry.lineIndex,
      pointIndex
    )
  }

  if (command === 'line-start' || command === 'line-end') {
    const pointIndex =
      command === 'line-start' ? 0 : currentEntry.line.points.length - 1
    return makeLineCursor(
      cursor,
      currentEntry.line,
      currentEntry.lineIndex,
      pointIndex
    )
  }

  let extreme = {
    entry: available[0]!,
    pointIndex: 0,
    timestampMs: available[0]!.line.points[0]!.timestampMs,
    timestampNs: pointTimestampNs(available[0]!.line.points[0]!),
  }
  for (const entry of available) {
    for (
      let pointIndex = 0;
      pointIndex < entry.line.points.length;
      pointIndex++
    ) {
      const point = entry.line.points[pointIndex]!
      const timestampMs = point.timestampMs
      const timestampNs = pointTimestampNs(point)
      const isBetter =
        command === 'global-start'
          ? timestampNs < extreme.timestampNs
          : timestampNs > extreme.timestampNs
      if (isBetter) extreme = { entry, pointIndex, timestampMs, timestampNs }
    }
  }
  return makeLineCursor(
    cursor,
    extreme.entry.line,
    extreme.entry.lineIndex,
    extreme.pointIndex
  )
}

export function lineCursorCommandForKey(
  event: KeyboardModifiers
): LineCursorCommand | null {
  if (event.altKey) return null
  const chartBoundary = event.ctrlKey || event.metaKey
  if (chartBoundary && event.key !== 'Home' && event.key !== 'End') return null

  if (event.key === 'ArrowLeft') return 'previous-point'
  if (event.key === 'ArrowRight') return 'next-point'
  if (event.key === 'ArrowUp') return 'previous-line'
  if (event.key === 'ArrowDown') return 'next-line'
  if (event.key === 'Home') {
    return chartBoundary ? 'global-start' : 'line-start'
  }
  if (event.key === 'End') return chartBoundary ? 'global-end' : 'line-end'
  return null
}

function makeOrderedCursor(
  current: OrderedCursor | null,
  keys: readonly CursorKey[],
  index: number
): OrderedCursor {
  const key = keys[index]!
  if (current && sameKey(current.key, key) && current.index === index) {
    return current
  }
  return { key, index }
}

export function reconcileOrderedCursor(
  keys: readonly CursorKey[],
  current: OrderedCursor | null
): OrderedCursor | null {
  if (keys.length === 0) return null
  if (current === null) return makeOrderedCursor(null, keys, 0)

  const keyedIndex = keys.findIndex(key => sameKey(key, current.key))
  const index =
    keyedIndex >= 0 ? keyedIndex : clamp(current.index, keys.length - 1)
  return makeOrderedCursor(current, keys, index)
}

export function moveOrderedCursor(
  keys: readonly CursorKey[],
  current: OrderedCursor | null,
  command: OrderedCursorCommand
): OrderedCursor | null {
  const cursor = reconcileOrderedCursor(keys, current)
  if (cursor === null) return null

  let index = cursor.index
  if (command === 'previous-item') index--
  if (command === 'next-item') index++
  if (command === 'first-item') index = 0
  if (command === 'last-item') index = keys.length - 1
  return makeOrderedCursor(cursor, keys, clamp(index, keys.length - 1))
}

export function orderedCursorCommandForKey(
  event: KeyboardModifiers
): OrderedCursorCommand | null {
  if (event.altKey || event.ctrlKey || event.metaKey) return null
  if (event.key === 'ArrowLeft') return 'previous-item'
  if (event.key === 'ArrowRight') return 'next-item'
  if (event.key === 'Home') return 'first-item'
  if (event.key === 'End') return 'last-item'
  return null
}

function keyIndex(
  keys: readonly CursorKey[],
  key: CursorKey,
  fallback: number
): number {
  const found = keys.findIndex(candidate => sameKey(candidate, key))
  return found >= 0 ? found : clamp(fallback, keys.length - 1)
}

function makeGridCursor(
  current: GridCursor | null,
  columns: readonly CursorKey[],
  rows: readonly CursorKey[],
  columnIndex: number,
  rowIndex: number
): GridCursor {
  const columnKey = columns[columnIndex]!
  const rowKey = rows[rowIndex]!
  if (
    current &&
    sameKey(current.columnKey, columnKey) &&
    sameKey(current.rowKey, rowKey) &&
    current.columnIndex === columnIndex &&
    current.rowIndex === rowIndex
  ) {
    return current
  }
  return { columnKey, rowKey, columnIndex, rowIndex }
}

export function gridCursorAt(
  columns: readonly CursorKey[],
  rows: readonly CursorKey[],
  columnKey?: CursorKey | null,
  rowKey?: CursorKey | null
): GridCursor | null {
  if (columns.length === 0 || rows.length === 0) return null
  const columnIndex =
    columnKey == null
      ? 0
      : Math.max(
          0,
          columns.findIndex(key => sameKey(key, columnKey))
        )
  const rowIndex =
    rowKey == null
      ? 0
      : Math.max(
          0,
          rows.findIndex(key => sameKey(key, rowKey))
        )
  return makeGridCursor(null, columns, rows, columnIndex, rowIndex)
}

export function reconcileGridCursor(
  columns: readonly CursorKey[],
  rows: readonly CursorKey[],
  current: GridCursor | null
): GridCursor | null {
  if (columns.length === 0 || rows.length === 0) return null
  if (current === null) return makeGridCursor(null, columns, rows, 0, 0)
  const columnIndex = keyIndex(columns, current.columnKey, current.columnIndex)
  const rowIndex = keyIndex(rows, current.rowKey, current.rowIndex)
  return makeGridCursor(current, columns, rows, columnIndex, rowIndex)
}

export function moveGridCursor(
  columns: readonly CursorKey[],
  rows: readonly CursorKey[],
  current: GridCursor | null,
  command: GridCursorCommand
): GridCursor | null {
  const cursor = reconcileGridCursor(columns, rows, current)
  if (cursor === null) return null

  let columnIndex = cursor.columnIndex
  let rowIndex = cursor.rowIndex
  if (command === 'previous-column') columnIndex--
  if (command === 'next-column') columnIndex++
  if (command === 'previous-row') rowIndex--
  if (command === 'next-row') rowIndex++
  if (command === 'row-start') columnIndex = 0
  if (command === 'row-end') columnIndex = columns.length - 1
  if (command === 'grid-start') {
    columnIndex = 0
    rowIndex = 0
  }
  if (command === 'grid-end') {
    columnIndex = columns.length - 1
    rowIndex = rows.length - 1
  }
  return makeGridCursor(
    cursor,
    columns,
    rows,
    clamp(columnIndex, columns.length - 1),
    clamp(rowIndex, rows.length - 1)
  )
}

export function gridCursorCommandForKey(
  event: KeyboardModifiers
): GridCursorCommand | null {
  if (event.altKey) return null
  const gridBoundary = event.ctrlKey || event.metaKey
  if (gridBoundary && event.key !== 'Home' && event.key !== 'End') return null

  if (event.key === 'ArrowLeft') return 'previous-column'
  if (event.key === 'ArrowRight') return 'next-column'
  if (event.key === 'ArrowUp') return 'previous-row'
  if (event.key === 'ArrowDown') return 'next-row'
  if (event.key === 'Home') return gridBoundary ? 'grid-start' : 'row-start'
  if (event.key === 'End') return gridBoundary ? 'grid-end' : 'row-end'
  return null
}

export function isChartActivationKey(key: string): boolean {
  return key === 'Enter' || key === ' '
}

export function normalizeChartAnnouncement(value: string): string {
  return value
    .replace(/\s+/g, ' ')
    .replace(/\s+([,.;:])/g, '$1')
    .trim()
}
