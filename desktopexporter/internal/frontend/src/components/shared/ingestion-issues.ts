import type { Rejection } from '@/types/api-types'

/** Latest present Unix-nanosecond timestamp, without conversion to Number. */
export function latestIssueTime(
  times: readonly (bigint | null)[]
): bigint | null {
  return times.reduce<bigint | null>(
    (latest, time) =>
      time !== null && (latest === null || time > latest) ? time : latest,
    null
  )
}

/** Card totals sum occurrences by reason; lastSeen is the latest source time. */
export function groupRejections(rejections: readonly Rejection[]) {
  const groups = new Map<string, Rejection[]>()
  for (const rejection of rejections) {
    const group = groups.get(rejection.kind)
    if (group) group.push(rejection)
    else groups.set(rejection.kind, [rejection])
  }
  return [...groups].map(([kind, entries]) => ({
    kind,
    occurrences: entries.reduce((sum, entry) => sum + entry.occurrences, 0),
    lastSeen: latestIssueTime(entries.map(entry => entry.lastSeen)),
    signals: [...new Set(entries.map(entry => entry.signal))],
    samples: entries.flatMap(entry => entry.samples),
  }))
}
