import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import type { TraceSummary } from '@/types/api-types'
import {
  formatDateTime,
  formatDateTimeMs,
  formatDateTimeRangeLabel,
  formatDuration,
  formatDurationParts,
  formatEditableDateTime,
  formatTimestamp,
  formatTimestampParts,
  formatTimezoneLabel,
  getLocalTimezoneName,
  getOffset,
  loadRecentTimeRanges,
  MAX_RECENT_TIME_RANGES,
  normalizeTimezone,
  parseDateTimeInTimezone,
  parseDuration,
  recordRecentTimeRange,
  timezoneOffsetMinutes,
  traceSummaryDurationNs,
  type RecentTimeRange,
} from './time'

const winterDate = new Date('2024-01-15T08:30:00.123456789Z')
const winterMs = winterDate.getTime()
const winterNs = BigInt(winterMs) * 1_000_000n + 456_789n
const newYork = normalizeTimezone('America/New_York')
if (!newYork) throw new Error('Test runtime lacks America/New_York')

const oneUsNs = 1_000n
const oneMsNs = 1_000_000n
const oneSecondNs = 1_000_000_000n
const oneMinuteNs = 60_000_000_000n
const oneHourNs = 3_600_000_000_000n

// Accepts string to mirror raw JSON payloads, which the implementation parses at runtime.
function makeSummary(
  durationNs: bigint | string | null | undefined
): TraceSummary {
  return {
    traceID: 't1',
    hasRootSpan: true,
    startTime: 0n,
    durationNs: durationNs as unknown as bigint | null,
    spanCount: 1,
    errorCount: 0,
  }
}

function createFakeStorage() {
  const store = new Map<string, string>()
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key),
  }
}

beforeEach(() => {
  vi.stubEnv('TZ', 'UTC')
  vi.stubGlobal('localStorage', createFakeStorage())
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

describe('parseDuration', () => {
  it('parses plain numeric strings as raw nanoseconds', () => {
    expect(parseDuration('12345')).toBe(12345n)
  })

  it('parses nanoseconds', () => {
    expect(parseDuration('100ns')).toBe(100n)
  })

  it('parses microseconds with the us suffix', () => {
    expect(parseDuration('5us')).toBe(5000n)
  })

  it('parses microseconds with the µs suffix', () => {
    expect(parseDuration('5µs')).toBe(5000n)
  })

  it('parses milliseconds', () => {
    expect(parseDuration('3ms')).toBe(3_000_000n)
  })

  it('parses seconds', () => {
    expect(parseDuration('2s')).toBe(2_000_000_000n)
  })

  it('parses minutes with the m suffix', () => {
    expect(parseDuration('2m')).toBe(2n * oneMinuteNs)
  })

  it('parses minutes with the min suffix', () => {
    expect(parseDuration('1min')).toBe(oneMinuteNs)
  })

  it('parses hours', () => {
    expect(parseDuration('1h')).toBe(oneHourNs)
  })

  it('parses decimal hours by rounding to nanoseconds', () => {
    expect(parseDuration('1.5h')).toBe(5_400_000_000_000n)
  })

  it('parses decimal milliseconds', () => {
    expect(parseDuration('0.5ms')).toBe(500_000n)
  })

  it('tolerates surrounding whitespace', () => {
    expect(parseDuration('  500 ms  ')).toBe(500_000_000n)
    expect(parseDuration('5\tms')).toBe(5_000_000n)
  })

  it('is case-insensitive for units', () => {
    expect(parseDuration('1H')).toBe(oneHourNs)
    expect(parseDuration('2MS')).toBe(2n * oneMsNs)
  })

  it('returns null for empty input', () => {
    expect(parseDuration('')).toBeNull()
    expect(parseDuration('   ')).toBeNull()
  })

  it('returns null for garbage input', () => {
    expect(parseDuration('abc')).toBeNull()
    expect(parseDuration('1x')).toBeNull()
    expect(parseDuration('1.2.3ms')).toBeNull()
  })

  it('returns null for negative numbers', () => {
    expect(parseDuration('-5ms')).toBeNull()
    expect(parseDuration('-12345')).toBeNull()
  })

  it('returns zero for zero durations', () => {
    expect(parseDuration('0s')).toBe(0n)
  })
})

describe('formatDurationParts', () => {
  it('formats nanoseconds at the ns scale', () => {
    expect(formatDurationParts(0n)).toEqual({ value: '0', unit: 'ns' })
    expect(formatDurationParts(500n)).toEqual({ value: '500', unit: 'ns' })
  })

  it('formats microseconds at the μs scale', () => {
    expect(formatDurationParts(oneUsNs)).toEqual({ value: '1.000', unit: 'μs' })
    expect(formatDurationParts(1_500n)).toEqual({ value: '1.500', unit: 'μs' })
  })

  it('formats milliseconds at the ms scale', () => {
    expect(formatDurationParts(oneMsNs)).toEqual({ value: '1.000', unit: 'ms' })
    expect(formatDurationParts(1_500_000n)).toEqual({
      value: '1.500',
      unit: 'ms',
    })
  })

  it('formats seconds at the s scale', () => {
    expect(formatDurationParts(oneSecondNs)).toEqual({
      value: '1.000',
      unit: 's',
    })
    expect(formatDurationParts(2n * oneSecondNs)).toEqual({
      value: '2.000',
      unit: 's',
    })
  })

  it('formats minute-long durations in seconds', () => {
    expect(formatDurationParts(oneMinuteNs)).toEqual({
      value: '60.000',
      unit: 's',
    })
  })

  it('formats hour-long durations in seconds', () => {
    expect(formatDurationParts(oneHourNs)).toEqual({
      value: '3600.000',
      unit: 's',
    })
  })

  it('rolls over just below the next unit', () => {
    expect(formatDurationParts(999_999n)).toEqual({
      value: '999.999',
      unit: 'μs',
    })
    expect(formatDurationParts(999_999_999n)).toEqual({
      value: '1000.000',
      unit: 'ms',
    })
  })
})

describe('formatDuration', () => {
  it('concatenates the value and the unit', () => {
    expect(formatDuration(1_500_000n)).toBe('1.500 ms')
    expect(formatDuration(2n * oneSecondNs)).toBe('2.000 s')
  })

  it('uses nanoseconds for zero', () => {
    expect(formatDuration(0n)).toBe('0 ns')
  })
})

describe('traceSummaryDurationNs', () => {
  it('returns a positive bigint unchanged', () => {
    expect(traceSummaryDurationNs(makeSummary(12345n))).toBe(12345n)
  })

  it('parses a string duration into nanoseconds', () => {
    expect(traceSummaryDurationNs(makeSummary('12345'))).toBe(12345n)
  })

  it('returns undefined for null', () => {
    expect(traceSummaryDurationNs(makeSummary(null))).toBeUndefined()
  })

  it('returns undefined for undefined', () => {
    expect(traceSummaryDurationNs(makeSummary(undefined))).toBeUndefined()
  })

  it('returns undefined for negative durations', () => {
    expect(traceSummaryDurationNs(makeSummary(-1n))).toBeUndefined()
    expect(traceSummaryDurationNs(makeSummary('-1'))).toBeUndefined()
  })

  it('returns zero for a zero duration', () => {
    expect(traceSummaryDurationNs(makeSummary(0n))).toBe(0n)
  })
})

describe('formatDateTimeMs', () => {
  it('returns the UTC wall-clock string and timezone label', () => {
    const result = formatDateTimeMs(winterMs, 'UTC')
    expect(result.dateTime).toContain('2024-01-15')
    expect(result.dateTime).toContain('08:30:00')
    expect(result.timezone).toBe('UTC')
  })

  it('formats in a named IANA timezone', () => {
    const result = formatDateTimeMs(winterMs, newYork)
    expect(result.dateTime).toContain('2024-01-15')
    expect(result.dateTime).toContain('03:30:00')
    expect(result.timezone).toBe('EST')
  })
})

describe('formatDateTime', () => {
  it('formats a UTC minute-resolution timestamp', () => {
    const formatted = formatDateTime(winterMs, 'UTC', 'minutes')
    expect(formatted).toContain('2024-01-15')
    expect(formatted).toContain('08:30')
    expect(formatted).toContain('UTC')
    expect(formatted).not.toContain('.')
  })

  it('includes seconds when asked for second resolution', () => {
    const formatted = formatDateTime(winterMs, 'UTC', 'seconds')
    expect(formatted).toContain('08:30:00')
    expect(formatted).not.toContain('.')
  })
})

describe('formatTimestamp', () => {
  it('formats minute resolution without fractional seconds', () => {
    const formatted = formatTimestamp(winterNs, 'UTC', 'minutes')
    expect(formatted).toContain('2024-01-15')
    expect(formatted).toContain('08:30')
    expect(formatted).toContain('UTC')
    expect(formatted).not.toContain('.')
  })

  it('includes seconds at second resolution', () => {
    const formatted = formatTimestamp(winterNs, 'UTC', 'seconds')
    expect(formatted).toContain('08:30:00')
    expect(formatted).not.toMatch(/\.\d+\sUTC/)
  })

  it('includes milliseconds at millisecond resolution', () => {
    const formatted = formatTimestamp(winterNs, 'UTC', 'milliseconds')
    expect(formatted).toContain('.123')
  })

  it('includes microseconds at microsecond resolution', () => {
    const formatted = formatTimestamp(winterNs, 'UTC', 'microseconds')
    expect(formatted).toContain('.456789')
  })

  it('includes nanoseconds at nanosecond resolution', () => {
    const formatted = formatTimestamp(winterNs, 'UTC', 'nanoseconds')
    expect(formatted).toContain('.456789789')
  })
})

describe('formatTimestampParts', () => {
  it('splits the wall-clock string from the timezone label', () => {
    const parts = formatTimestampParts(winterNs, 'UTC')
    expect(parts.value).toContain('2024-01-15')
    expect(parts.value).toContain('08:30:00')
    expect(parts.unit).toBe('UTC')
  })
})

describe('formatDateTimeRangeLabel', () => {
  const startMs = new Date('2024-01-15T07:00:00Z').getTime()
  const endMs = new Date('2024-01-15T08:30:00Z').getTime()

  it('labels a range with start and end times', () => {
    const formatted = formatDateTimeRangeLabel(startMs, endMs, 'UTC')
    expect(formatted).toContain('2024-01-15')
    expect(formatted).toContain('07:00:00')
    expect(formatted).toContain('08:30:00')
  })

  it('includes the timezone label when requested', () => {
    const formatted = formatDateTimeRangeLabel(startMs, endMs, 'UTC', {
      includeTimezone: true,
    })
    expect(formatted).toContain('UTC')
  })

  it('labels a range starting at zero as Before the end time', () => {
    const formatted = formatDateTimeRangeLabel(0, endMs, 'UTC')
    expect(formatted).toMatch(/^Before /)
    expect(formatted).toContain('08:30:00')
  })

  it('includes the timezone label for a zero-start range when requested', () => {
    const formatted = formatDateTimeRangeLabel(0, endMs, 'UTC', {
      includeTimezone: true,
    })
    expect(formatted).toContain('UTC')
  })

  it('labels both sides when a named-zone range crosses a DST change', () => {
    const formatted = formatDateTimeRangeLabel(
      new Date('2026-03-08T06:30:00Z').getTime(),
      new Date('2026-03-08T07:30:00Z').getTime(),
      newYork,
      { includeTimezone: true }
    )
    expect(formatted).toContain('01:30:00.000 EST')
    expect(formatted).toContain('03:30:00.000 EDT')
  })
})

describe('getOffset', () => {
  it('returns 0 for a point at the start', () => {
    expect(getOffset(0n, 100n, 0n)).toBe(0)
  })

  it('returns 100 for a point at the end', () => {
    expect(getOffset(0n, 100n, 100n)).toBe(100)
  })

  it('returns 50 for a midpoint', () => {
    expect(getOffset(0n, 100n, 50n)).toBe(50)
  })

  it('returns 0 when the interval is zero or negative', () => {
    expect(getOffset(100n, 100n, 150n)).toBe(0)
    expect(getOffset(200n, 100n, 150n)).toBe(0)
  })

  it('computes offsets beyond the interval without clamping', () => {
    expect(getOffset(0n, 100n, 250n)).toBe(250)
  })
})

describe('formatTimezoneLabel', () => {
  it('returns UTC for the UTC timezone', () => {
    expect(formatTimezoneLabel('UTC', winterDate)).toBe('UTC')
  })

  it('returns a short label for the local timezone', () => {
    vi.stubEnv('TZ', 'America/Los_Angeles')
    const laWinter = new Date('2024-01-15T08:30:00Z')
    expect(formatTimezoneLabel('local', laWinter)).toBe('PST')
  })
})

describe('named timezone support', () => {
  it('canonicalizes valid names and rejects unknown zones', () => {
    expect(normalizeTimezone('America/New_York')).toBe('America/New_York')
    expect(normalizeTimezone('Mars/Standard')).toBeNull()
    expect(normalizeTimezone('+01:00')).toBeNull()
  })

  it('resolves the offset in force at each instant', () => {
    expect(timezoneOffsetMinutes(newYork, winterMs)).toBe(-300)
    expect(
      timezoneOffsetMinutes(newYork, new Date('2024-07-15T08:30:00Z').getTime())
    ).toBe(-240)
  })

  it('parses an absolute wall-clock time in the selected zone', () => {
    const result = parseDateTimeInTimezone(
      '2026-01-15, 07:00:00.000',
      newYork,
      new Date('2026-01-16T00:00:00Z').getTime()
    )
    expect(result).toEqual({
      success: true,
      timestamp: new Date('2026-01-15T12:00:00Z').getTime(),
    })
  })

  it('keeps elapsed hours absolute but calendar days on the selected wall clock', () => {
    const now = new Date('2026-03-08T16:00:00Z').getTime()
    expect(parseDateTimeInTimezone('1 hour ago', newYork, now)).toEqual({
      success: true,
      timestamp: new Date('2026-03-08T15:00:00Z').getTime(),
    })
    expect(parseDateTimeInTimezone('1 day ago', newYork, now)).toEqual({
      success: true,
      timestamp: new Date('2026-03-07T17:00:00Z').getTime(),
    })
  })

  it('rejects a wall time skipped by spring-forward', () => {
    const result = parseDateTimeInTimezone(
      '2026-03-08 02:30',
      newYork,
      new Date('2026-03-09T00:00:00Z').getTime()
    )
    expect(result).toEqual({
      success: false,
      error:
        'This time does not exist in America/New_York because the clocks changed',
    })
  })

  it('rejects an ambiguous wall time unless an offset is explicit', () => {
    const ambiguous = parseDateTimeInTimezone(
      '2026-11-01 01:30',
      newYork,
      new Date('2026-11-02T00:00:00Z').getTime()
    )
    expect(ambiguous).toEqual({
      success: false,
      error:
        'This time occurs twice in America/New_York; include an explicit UTC offset',
    })

    const explicit = parseDateTimeInTimezone(
      '2026-11-01T01:30:00-04:00',
      newYork
    )
    expect(explicit).toEqual({
      success: true,
      timestamp: new Date('2026-11-01T05:30:00Z').getTime(),
    })
  })

  it('formats editable values with the offset that disambiguates overlaps', () => {
    expect(
      formatEditableDateTime(
        new Date('2026-11-01T05:30:00Z').getTime(),
        newYork
      )
    ).toBe('2026-11-01T01:30:00.000-04:00')
    expect(
      formatEditableDateTime(
        new Date('2026-11-01T06:30:00Z').getTime(),
        newYork
      )
    ).toBe('2026-11-01T01:30:00.000-05:00')
  })

  it('preserves historical second-precision offsets', () => {
    const monrovia = normalizeTimezone('Africa/Monrovia')
    if (!monrovia) throw new Error('Test runtime lacks Africa/Monrovia')
    const result = parseDateTimeInTimezone(
      '1971-01-01 00:00:00',
      monrovia,
      new Date('1971-01-02T00:00:00Z').getTime()
    )
    expect(result).toEqual({
      success: true,
      timestamp: new Date('1971-01-01T00:44:30Z').getTime(),
    })
    if (!result.success) throw new Error(result.error)
    const editable = formatEditableDateTime(result.timestamp, monrovia)
    expect(editable).toBe('1971-01-01T00:00:00.000-00:44:30')
    expect(parseDateTimeInTimezone(editable, monrovia)).toEqual(result)
  })

  it('parses explicit negative-zero-hour offsets without losing their sign', () => {
    expect(
      parseDateTimeInTimezone('2026-01-01T00:00:00.000-00:30', newYork)
    ).toEqual({
      success: true,
      timestamp: new Date('2026-01-01T00:30:00Z').getTime(),
    })
    expect(
      parseDateTimeInTimezone('2026-01-01T00:00:00.000-0030', newYork)
    ).toEqual({
      success: true,
      timestamp: new Date('2026-01-01T00:30:00Z').getTime(),
    })
  })

  it('round-trips named-zone instants in years below 100', () => {
    const timestamp = new Date('0050-01-01T12:00:00Z').getTime()
    const editable = formatEditableDateTime(timestamp, newYork)
    expect(parseDateTimeInTimezone(editable, newYork)).toEqual({
      success: true,
      timestamp,
    })
  })
})

describe('getLocalTimezoneName', () => {
  it('returns a non-empty long timezone name', () => {
    const name = getLocalTimezoneName()
    expect(typeof name).toBe('string')
    expect(name.length).toBeGreaterThan(0)
    expect(name).not.toBe('Local Time')
  })
})

describe('loadRecentTimeRanges', () => {
  it('returns an empty array when nothing is stored', () => {
    expect(loadRecentTimeRanges()).toEqual([])
  })

  it('returns an empty array for malformed JSON', () => {
    localStorage.setItem('datetime-filter-recent', 'not json')
    expect(loadRecentTimeRanges()).toEqual([])
  })

  it('returns an empty array for non-array JSON', () => {
    localStorage.setItem('datetime-filter-recent', '{}')
    expect(loadRecentTimeRanges()).toEqual([])
  })

  it('loads and sorts stored ranges by usedAt descending', () => {
    const ranges: RecentTimeRange[] = [
      { start: 1, end: 2, usedAt: 100 },
      { start: 3, end: 4, usedAt: 300 },
      { start: 5, end: 6, usedAt: 200 },
    ]
    localStorage.setItem('datetime-filter-recent', JSON.stringify(ranges))
    expect(loadRecentTimeRanges()).toEqual([
      { start: 3, end: 4, usedAt: 300 },
      { start: 5, end: 6, usedAt: 200 },
      { start: 1, end: 2, usedAt: 100 },
    ])
  })

  it('trims stored ranges to MAX_RECENT_TIME_RANGES and persists the trimmed list', () => {
    const ranges: RecentTimeRange[] = Array.from({ length: 7 }, (_, i) => ({
      start: i,
      end: i + 1,
      usedAt: i,
    }))
    localStorage.setItem('datetime-filter-recent', JSON.stringify(ranges))
    const loaded = loadRecentTimeRanges()
    expect(loaded).toHaveLength(MAX_RECENT_TIME_RANGES)
    expect(loaded[0].usedAt).toBe(6)
    expect(loaded[MAX_RECENT_TIME_RANGES - 1].usedAt).toBe(2)
    expect(
      JSON.parse(localStorage.getItem('datetime-filter-recent')!)
    ).toHaveLength(MAX_RECENT_TIME_RANGES)
  })
})

describe('recordRecentTimeRange', () => {
  it('records a new range and stores it', () => {
    recordRecentTimeRange(1, 2, 1000)
    expect(loadRecentTimeRanges()).toEqual([{ start: 1, end: 2, usedAt: 1000 }])
  })

  it('updates the usedAt of an existing range instead of duplicating it', () => {
    recordRecentTimeRange(1, 2, 1000)
    recordRecentTimeRange(1, 2, 2000)
    expect(loadRecentTimeRanges()).toEqual([{ start: 1, end: 2, usedAt: 2000 }])
  })

  it('keeps the most recent ranges up to the maximum', () => {
    for (let i = 0; i < MAX_RECENT_TIME_RANGES + 2; i++) {
      recordRecentTimeRange(i, i + 1, i)
    }
    const ranges = loadRecentTimeRanges()
    expect(ranges).toHaveLength(MAX_RECENT_TIME_RANGES)
    expect(ranges.map(r => r.usedAt)).toEqual([6, 5, 4, 3, 2])
  })

  it('re-sorts ranges by usedAt after recording', () => {
    recordRecentTimeRange(1, 2, 100)
    recordRecentTimeRange(3, 4, 300)
    recordRecentTimeRange(5, 6, 200)
    const ranges = loadRecentTimeRanges()
    expect(ranges.map(r => r.usedAt)).toEqual([300, 200, 100])
  })
})
