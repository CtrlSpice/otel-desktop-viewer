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
  externalIdentity(): string | null
  syncExternal(current: Cursor | null): Cursor | null
}

function keyIdentity(key: CursorKey | null): string | null {
  return key === null ? null : `${typeof key}:${String(key)}`
}

function lineCursorIdentity(cursor: LineCursor | null): string | null {
  return cursor
    ? `${cursor.lineKey}\u0000${keyIdentity(cursor.pointKey)}`
    : null
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

  let lastExternalIdentity: string | null | undefined
  $effect(() => {
    const identity = config.externalIdentity()
    if (identity === lastExternalIdentity) return
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
