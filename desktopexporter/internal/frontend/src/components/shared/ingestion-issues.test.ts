import { describe, expect, it } from 'vitest'
import type { Rejection } from '@/types/api-types'
import { groupRejections, latestIssueTime } from './ingestion-issues'

describe('ingestion issue summaries', () => {
  it('keeps separate reasons and combines their counts, signals and latest time', () => {
    const earlier = 1792692000000000001n
    const later = earlier + 1n
    const sample = { traceID: 'a'.repeat(32), spanID: 'b'.repeat(16) }
    const rejections: Rejection[] = [
      {
        signal: 'logs',
        kind: 'receiver_recursion_depth',
        occurrences: 3,
        samples: [],
        firstSeen: earlier,
        lastSeen: later,
      },
      {
        signal: 'traces',
        kind: 'span_already_stored',
        occurrences: 2,
        samples: [sample],
        firstSeen: earlier,
        lastSeen: earlier,
      },
      {
        signal: 'metrics',
        kind: 'receiver_recursion_depth',
        occurrences: 1,
        samples: [],
        firstSeen: earlier,
        lastSeen: earlier,
      },
      {
        signal: 'traces',
        kind: 'span_refused',
        occurrences: 7,
        samples: [],
        firstSeen: earlier,
        lastSeen: null,
      },
    ]
    const original = structuredClone(rejections)
    expect(groupRejections(rejections)).toEqual([
      {
        kind: 'receiver_recursion_depth',
        occurrences: 4,
        lastSeen: later,
        signals: ['logs', 'metrics'],
        samples: [],
      },
      {
        kind: 'span_already_stored',
        occurrences: 2,
        lastSeen: earlier,
        signals: ['traces'],
        samples: [sample],
      },
      {
        kind: 'span_refused',
        occurrences: 7,
        lastSeen: null,
        signals: ['traces'],
        samples: [],
      },
    ])
    expect(rejections).toEqual(original)
  })

  it('preserves unknown reason names without treating them as object properties', () => {
    const rejections: Rejection[] = ['constructor', '__proto__'].map(kind => ({
      signal: 'traces',
      kind,
      occurrences: 1,
      samples: [],
      firstSeen: null,
      lastSeen: null,
    }))
    expect(groupRejections(rejections).map(group => group.kind)).toEqual([
      'constructor',
      '__proto__',
    ])
    expect(groupRejections([])).toEqual([])
  })

  it('selects the latest present time without inventing a timestamp', () => {
    expect(latestIssueTime([])).toBeNull()
    expect(latestIssueTime([null, null])).toBeNull()
    expect(latestIssueTime([-2n, null, -1n])).toBe(-1n)
    expect(latestIssueTime([null, -1n, 0n])).toBe(0n)
  })
})
