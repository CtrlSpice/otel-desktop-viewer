import {
  gridCursorAt,
  moveGridCursor,
  moveLineCursor,
  moveOrderedCursor,
  reconcileGridCursor,
  reconcileLineCursor,
  reconcileOrderedCursor,
  type CursorKey,
  type GridCursor,
  type GridCursorCommand,
  type KeyboardLine,
  type LineCursor,
  type LineCursorCommand,
  type OrderedCursor,
  type OrderedCursorCommand,
} from './chart-keyboard-cursor'

export type ChartKeyboardCursorState<Cursor, Command> = {
  readonly current: Cursor | null
  readonly focused: boolean
  setFocused(focused: boolean): void
  move(command: Command): void
}

type CursorStateConfig<Cursor, Command> = {
  initial(): Cursor | null
  reconcile(current: Cursor | null): Cursor | null
  move(current: Cursor | null, command: Command): Cursor | null
  externalIdentity(): CursorIdentity | null
  syncExternal(current: Cursor | null): Cursor | null
}

type CursorIdentity =
  readonly [key: CursorKey] | readonly [first: CursorKey, second: CursorKey]

function sameIdentity(
  a: CursorIdentity | null | undefined,
  b: CursorIdentity | null
): boolean {
  if (a === null || a === undefined || b === null) return a === b
  return (
    a.length === b.length &&
    Object.is(a[0], b[0]) &&
    (a.length === 1 || Object.is(a[1], b[1]))
  )
}

function keyIdentity(key: CursorKey | null): CursorIdentity | null {
  return key === null ? null : [key]
}

function lineCursorIdentity(cursor: LineCursor | null): CursorIdentity | null {
  return cursor ? [cursor.lineKey, cursor.pointKey] : null
}

function createCursorState<Cursor, Command>(
  config: CursorStateConfig<Cursor, Command>
): ChartKeyboardCursorState<Cursor, Command> {
  let cursor = $state<Cursor | null>(null)
  let focused = $state(false)
  let active = $derived.by(() =>
    config.reconcile(focused ? cursor : (config.initial() ?? cursor))
  )

  $effect(() => {
    const next = active
    if (focused && next !== cursor) cursor = next
  })

  let lastExternalIdentity: CursorIdentity | null | undefined
  $effect(() => {
    const identity = config.externalIdentity()
    if (sameIdentity(lastExternalIdentity, identity)) return
    lastExternalIdentity = identity
    if (!focused || identity === null) return

    const next = config.syncExternal(active)
    if (next) cursor = next
  })

  return {
    get current() {
      return active
    },
    get focused() {
      return focused
    },
    setFocused(next: boolean) {
      focused = next
      if (next) cursor = config.initial() ?? config.reconcile(cursor)
    },
    move(command: Command) {
      cursor = config.move(active, command)
    },
  }
}

export function createLineChartKeyboardCursor(
  lines: () => readonly KeyboardLine[],
  initial: () => LineCursor | null
): ChartKeyboardCursorState<LineCursor, LineCursorCommand> {
  return createCursorState({
    initial,
    reconcile: current => reconcileLineCursor(lines(), current),
    move: (current, command) => moveLineCursor(lines(), current, command),
    externalIdentity: () => lineCursorIdentity(initial()),
    syncExternal: () => initial(),
  })
}

export function createGridChartKeyboardCursor(
  columns: () => readonly CursorKey[],
  rows: () => readonly CursorKey[],
  selectedColumn: () => CursorKey | null
): ChartKeyboardCursorState<GridCursor, GridCursorCommand> {
  const initial = () => {
    const column = selectedColumn()
    return column === null
      ? null
      : gridCursorAt(columns(), rows(), column, null)
  }

  return createCursorState({
    initial,
    reconcile: current => reconcileGridCursor(columns(), rows(), current),
    move: (current, command) =>
      moveGridCursor(columns(), rows(), current, command),
    externalIdentity: () => keyIdentity(selectedColumn()),
    syncExternal: current => {
      const column = selectedColumn()
      return column === null
        ? null
        : gridCursorAt(columns(), rows(), column, current?.rowKey ?? null)
    },
  })
}

export function createOrderedChartKeyboardCursor(
  keys: () => readonly CursorKey[],
  selectedKey: () => CursorKey | null
): ChartKeyboardCursorState<OrderedCursor, OrderedCursorCommand> {
  const cursorForSelection = (current: OrderedCursor | null) => {
    const selected = selectedKey()
    return selected === null
      ? null
      : reconcileOrderedCursor(keys(), {
          key: selected,
          index: current?.index ?? 0,
        })
  }

  return createCursorState({
    initial: () => cursorForSelection(null),
    reconcile: current => reconcileOrderedCursor(keys(), current),
    move: (current, command) => moveOrderedCursor(keys(), current, command),
    externalIdentity: () => keyIdentity(selectedKey()),
    syncExternal: cursorForSelection,
  })
}
