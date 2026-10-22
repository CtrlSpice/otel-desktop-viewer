import type { EvalExperiment } from './model'

export type EvalExperimentSort = 'started' | 'name'

function startedTimestamp(value: string | undefined): bigint | null {
  if (!value) return null
  const milliseconds = Date.parse(value)
  if (!Number.isFinite(milliseconds)) return null
  // Date.parse handles the offset but truncates sub-millisecond precision.
  const fraction = value.match(/\.(\d+)(?:Z|[+-]\d{2}:\d{2})$/i)?.[1] ?? ''
  const remainder = fraction.padEnd(9, '0').slice(3, 9)
  return BigInt(milliseconds) * 1_000_000n + BigInt(remainder)
}

export function compareEvalExperiments(
  a: Pick<EvalExperiment, 'id' | 'name' | 'startedAt'>,
  b: Pick<EvalExperiment, 'id' | 'name' | 'startedAt'>,
  sort: EvalExperimentSort,
  direction: 'asc' | 'desc'
): number {
  let comparison = 0
  if (sort === 'started') {
    const left = startedTimestamp(a.startedAt)
    const right = startedTimestamp(b.startedAt)
    // Unstarted or invalid dates stay last in either direction, not at epoch.
    if (left === null && right !== null) return 1
    if (right === null && left !== null) return -1
    if (left !== null && right !== null)
      comparison = left < right ? -1 : left > right ? 1 : 0
  } else comparison = a.name.localeCompare(b.name)
  return comparison
    ? direction === 'asc'
      ? comparison
      : -comparison
    : a.name.localeCompare(b.name) || a.id.localeCompare(b.id)
}
