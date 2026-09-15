type KeyDelta =
  | { kind: 'relative'; offset: number }
  | { kind: 'absolute'; position: 'first' | 'last' }

const PAGE_STEP = 10

export function keyDeltaFor(
  key: string,
  pageStep: number = PAGE_STEP
): KeyDelta | undefined {
  switch (key) {
    case 'ArrowDown':
    case 'j':
      return { kind: 'relative', offset: 1 }
    case 'ArrowUp':
    case 'k':
      return { kind: 'relative', offset: -1 }
    case 'PageDown':
      return { kind: 'relative', offset: pageStep }
    case 'PageUp':
      return { kind: 'relative', offset: -pageStep }
    case 'Home':
      return { kind: 'absolute', position: 'first' }
    case 'End':
      return { kind: 'absolute', position: 'last' }
    default:
      return undefined
  }
}

function resolveNextPos(
  delta: KeyDelta,
  currentPos: number,
  lastPos: number
): number {
  const raw =
    delta.kind === 'absolute'
      ? delta.position === 'first'
        ? 0
        : lastPos
      : currentPos + delta.offset
  return Math.max(0, Math.min(raw, lastPos))
}

function escapeForSelector(value: string): string {
  return typeof CSS !== 'undefined' && typeof CSS.escape === 'function'
    ? CSS.escape(value)
    : value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
}

export { escapeForSelector, resolveNextPos, type KeyDelta }
