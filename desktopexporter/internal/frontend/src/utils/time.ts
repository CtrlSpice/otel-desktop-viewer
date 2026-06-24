export type Timezone = 'local' | 'UTC'

export type FormattedDateTime = {
  dateTime: string
  timezone: string
}

type DateTimeResolution = 'minutes' | 'seconds' | 'milliseconds'
type TimestampResolution = DateTimeResolution | 'microseconds' | 'nanoseconds'

/** Short timezone label for UI chrome (UTC, PST, …). */
export function formatTimezoneLabel(
  timezone: Timezone,
  date: Date = new Date()
): string {
  if (timezone === 'UTC') return 'UTC'
  return (
    new Intl.DateTimeFormat('en', { timeZoneName: 'short' })
      .formatToParts(date)
      .find(part => part.type === 'timeZoneName')?.value ?? 'Local'
  )
}

function formatWallClock(
  date: Date,
  timezone: Timezone,
  resolution: TimestampResolution
): string {
  let options: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }

  switch (resolution) {
    case 'seconds':
      options.second = '2-digit'
      break
    case 'milliseconds':
    case 'microseconds':
    case 'nanoseconds':
      options.second = '2-digit'
      options.fractionalSecondDigits = 3
      break
  }

  if (timezone === 'UTC') {
    return date.toLocaleString('en-CA', { ...options, timeZone: 'UTC' })
  }
  return date.toLocaleString('en-CA', options)
}

/** Wall-clock instant to ms precision + short timezone label (split for headers/tables). */
export function formatDateTimeMs(
  ms: number,
  timezone: Timezone
): FormattedDateTime {
  const date = new Date(ms)
  return {
    dateTime: formatWallClock(date, timezone, 'milliseconds'),
    timezone: formatTimezoneLabel(timezone, date),
  }
}

// UI time: number = Unix ms (from Date.now(), time pickers, etc.)
export function formatDateTime(
  ms: number,
  timezone: Timezone,
  resolution: DateTimeResolution = 'minutes'
): string {
  const date = new Date(ms)
  const dateTime = formatWallClock(date, timezone, resolution)
  return `${dateTime} ${formatTimezoneLabel(timezone, date)}`
}

// Telemetry time: bigint = Unix nanoseconds (from backend OTLP data)
export function formatTimestamp(
  ns: bigint,
  timezone: Timezone,
  resolution: TimestampResolution = 'nanoseconds'
): string {
  let epochMs = Number(ns / 1_000_000n)
  let subMs = ns % 1_000_000n
  let date = new Date(epochMs)
  let formatted = `${formatWallClock(date, timezone, resolution)} ${formatTimezoneLabel(timezone, date)}`

  if (resolution === 'microseconds') {
    let micros = Number(subMs).toString().padStart(6, '0')
    return formatted.replace(/\.\d{3}(\s)/, `.${micros}$1`)
  }
  if (resolution === 'nanoseconds') {
    let nanos = Number(subMs).toString().padStart(6, '0')
    let extraNanos = Number(ns % 1000n)
      .toString()
      .padStart(3, '0')
    return formatted.replace(/\.\d{3}(\s)/, `.${nanos}${extraNanos}$1`)
  }
  return formatted
}

export function formatDateTimeRangeLabel(
  start: number,
  end: number,
  timezone: Timezone,
  options: { includeTimezone?: boolean } = {}
): string {
  const { includeTimezone = false } = options
  const tz = formatTimezoneLabel(timezone, new Date(end))

  if (start === 0) {
    const range = `Before ${formatDateTimeMs(end, timezone).dateTime}`
    return includeTimezone ? `${range} ${tz}` : range
  }

  const startLabel = formatDateTimeMs(start, timezone).dateTime
  const endLabel = formatDateTimeMs(end, timezone).dateTime
  const range = `${startLabel} - ${endLabel}`
  return includeTimezone ? `${range} ${tz}` : range
}

export function getLocalTimezoneName(): string {
  try {
    let timeZoneName = new Intl.DateTimeFormat('en', {
      timeZoneName: 'long',
    })
      .formatToParts(new Date())
      .find(part => part.type === 'timeZoneName')?.value

    return timeZoneName || 'Local Time'
  } catch (error) {
    return 'Local Time'
  }
}

// --- Duration formatting & parsing ---

import type { TraceSummary } from '@/types/api-types'
import { parseBigInt } from '@/utils/bigint'

/** Nanoseconds of trace coverage for list display/sort (server-precomputed). */
export function traceSummaryDurationNs(
  summary: TraceSummary
): bigint | undefined {
  const ns = summary.durationNs
  if (ns === null || ns === undefined) return undefined
  const bi = typeof ns === 'bigint' ? ns : parseBigInt(ns)
  return bi >= 0n ? bi : undefined
}

const DURATION_UNITS: Record<string, bigint> = {
  ns: 1n,
  us: 1_000n,
  '\u00b5s': 1_000n, // µs
  ms: 1_000_000n,
  s: 1_000_000_000n,
  m: 60_000_000_000n,
  min: 60_000_000_000n,
  h: 3_600_000_000_000n,
}

const DURATION_RE = /^(\d+(?:\.\d+)?)\s*(ns|us|µs|ms|s|min|m|h)$/i

/**
 * Parse a human-readable duration string into nanoseconds.
 * Accepts formats like "1s", "500ms", "2m", "1.5h", "100ns".
 * Plain numeric strings are treated as raw nanoseconds.
 * Returns null if the string cannot be parsed.
 */
export function parseDuration(input: string): bigint | null {
  const trimmed = input.trim()
  if (!trimmed) return null

  if (/^\d+$/.test(trimmed)) return BigInt(trimmed)

  const match = trimmed.match(DURATION_RE)
  if (!match) return null

  const [, numStr, unit] = match
  const multiplier = DURATION_UNITS[unit.toLowerCase()]
  if (multiplier === undefined) return null

  const num = parseFloat(numStr)
  if (!isFinite(num) || num < 0) return null

  if (Number.isInteger(num)) {
    return BigInt(num) * multiplier
  }
  return BigInt(Math.round(num * Number(multiplier)))
}

export function formatDuration(nanoseconds: bigint): string {
  const { value, unit } = formatDurationParts(nanoseconds)
  return unit ? `${value} ${unit}` : value
}

/** Value + unit for labeled duration display (e.g. drawer cards). */
export function formatDurationParts(nanoseconds: bigint): {
  value: string
  unit: string
} {
  if (nanoseconds >= 1_000_000_000n) {
    const seconds = Number(nanoseconds) / 1_000_000_000
    return { value: seconds.toFixed(3), unit: 's' }
  }
  if (nanoseconds >= 1_000_000n) {
    const ms = Number(nanoseconds) / 1_000_000
    return { value: ms.toFixed(3), unit: 'ms' }
  }
  if (nanoseconds >= 1000n) {
    const μs = Number(nanoseconds) / 1000
    return { value: μs.toFixed(3), unit: 'μs' }
  }
  return { value: String(Number(nanoseconds)), unit: 'ns' }
}

/** Datetime value + timezone suffix for labeled timestamp display. */
export function formatTimestampParts(
  ns: bigint,
  timezone: Timezone,
  resolution: TimestampResolution = 'nanoseconds'
): { value: string; unit: string } {
  const formatted = formatTimestamp(ns, timezone, resolution)
  const lastSpace = formatted.lastIndexOf(' ')
  if (lastSpace === -1) return { value: formatted, unit: '' }
  return {
    value: formatted.slice(0, lastSpace),
    unit: formatted.slice(lastSpace + 1),
  }
}

export function getOffset(
  startTime: bigint,
  endTime: bigint,
  point: bigint
): number {
  let totalNs = endTime - startTime
  if (totalNs <= 0n) return 0
  let offsetNs = point - startTime
  return Math.floor(Number((offsetNs * 100n) / totalNs))
}

// --- Recent time ranges (localStorage persistence) ---

const RECENT_STORAGE_KEY = 'datetime-filter-recent'

export const MAX_RECENT_TIME_RANGES = 5

export type RecentTimeRange = {
  start: number
  end: number
  usedAt: number
}

export function loadRecentTimeRanges(): RecentTimeRange[] {
  try {
    const saved = localStorage.getItem(RECENT_STORAGE_KEY)
    if (!saved) return []
    const parsed: unknown = JSON.parse(saved)
    if (!Array.isArray(parsed)) return []
    const rows = parsed as RecentTimeRange[]
    const sorted = [...rows].sort((a, b) => b.usedAt - a.usedAt)
    const trimmed = sorted.slice(0, MAX_RECENT_TIME_RANGES)
    if (trimmed.length < rows.length) {
      localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(trimmed))
    }
    return trimmed
  } catch {
    return []
  }
}

/** Add or bump a range in recents (dedupe by start/end). Persists to localStorage. */
export function recordRecentTimeRange(
  start: number,
  end: number,
  usedAt: number
): void {
  let recentTimeRanges = loadRecentTimeRanges()
  const existingIndex = recentTimeRanges.findIndex(
    e => e.start === start && e.end === end
  )

  if (existingIndex !== -1) {
    const updated = [...recentTimeRanges]
    updated[existingIndex] = { ...updated[existingIndex], usedAt }
    recentTimeRanges = updated
      .sort((a, b) => b.usedAt - a.usedAt)
      .slice(0, MAX_RECENT_TIME_RANGES)
  } else {
    recentTimeRanges = [{ start, end, usedAt }, ...recentTimeRanges]
      .sort((a, b) => b.usedAt - a.usedAt)
      .slice(0, MAX_RECENT_TIME_RANGES)
  }

  localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(recentTimeRanges))
}
