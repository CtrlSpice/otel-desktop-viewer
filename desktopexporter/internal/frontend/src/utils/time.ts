import * as chrono from 'chrono-node'

declare const ianaTimezoneBrand: unique symbol

export type IANATimezone = string & {
  readonly [ianaTimezoneBrand]: true
}
export type Timezone = 'local' | 'UTC' | IANATimezone

export type FormattedDateTime = {
  dateTime: string
  timezone: string
}

type DateTimeResolution = 'minutes' | 'seconds' | 'milliseconds'
type TimestampResolution = DateTimeResolution | 'microseconds' | 'nanoseconds'

export type ParsedDateTime =
  { success: true; timestamp: number } | { success: false; error: string }
export type WallClockDisambiguation = 'reject' | 'earlier' | 'later'

const zonedPartsFormatters = new Map<string, Intl.DateTimeFormat>()
const EXPLICIT_OFFSET_DATE_TIME =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,3}))?(Z|[+-](?:\d{2}:\d{2}(?::\d{2})?|\d{4}(?:\d{2})?))$/i

/** Validate an IANA name and return the runtime's canonical spelling. */
export function normalizeTimezone(value: string): Timezone | null {
  if (value === 'local') return 'local'
  // ECMA-402 accepts offset identifiers in some browsers, but DuckDB's ICU
  // timezone lookup expects a named zone. Fixed offsets are not IANA names.
  if (/^[+-]\d{2}(?::?\d{2})?$/.test(value)) return null
  try {
    const canonical = new Intl.DateTimeFormat('en', {
      timeZone: value,
    }).resolvedOptions().timeZone
    return canonical === 'UTC' ? 'UTC' : (canonical as IANATimezone)
  } catch {
    return null
  }
}

/** Named zones offered by this runtime. Manual entry remains available as a fallback. */
export function getSupportedTimezones(): IANATimezone[] {
  if (typeof Intl.supportedValuesOf !== 'function') return []
  const zones = new Set<IANATimezone>()
  for (const value of Intl.supportedValuesOf('timeZone')) {
    const canonical = normalizeTimezone(value)
    if (canonical && canonical !== 'local' && canonical !== 'UTC') {
      zones.add(canonical)
    }
  }
  return [...zones].sort()
}

/** Resolve the machine-following pseudo-zone to the current IANA name. */
export function resolveTimezoneName(timezone: Timezone): string {
  if (timezone !== 'local') return timezone
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

function zonedPartsFormatter(timezone: string): Intl.DateTimeFormat {
  let formatter = zonedPartsFormatters.get(timezone)
  if (!formatter) {
    formatter = new Intl.DateTimeFormat('en-CA', {
      timeZone: timezone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23',
    })
    zonedPartsFormatters.set(timezone, formatter)
  }
  return formatter
}

type WallClock = {
  year: number
  month: number
  day: number
  hour: number
  minute: number
  second: number
  millisecond: number
}

function wallClockAt(timestamp: number, timezone: string): WallClock {
  const values = new Map(
    zonedPartsFormatter(timezone)
      .formatToParts(new Date(timestamp))
      .filter(part => part.type !== 'literal')
      .map(part => [part.type, Number(part.value)])
  )
  return {
    year: values.get('year') ?? 0,
    month: values.get('month') ?? 0,
    day: values.get('day') ?? 0,
    hour: values.get('hour') ?? 0,
    minute: values.get('minute') ?? 0,
    second: values.get('second') ?? 0,
    millisecond: ((timestamp % 1000) + 1000) % 1000,
  }
}

function timezoneOffsetMilliseconds(
  timezone: Timezone,
  timestamp: number
): number {
  const name = resolveTimezoneName(timezone)
  if (name === 'UTC') return 0
  const wall = wallClockAt(timestamp, name)
  const wallAsUTC = wallClockEpoch({ ...wall, millisecond: 0 })
  const instantAtSecond = Math.floor(timestamp / 1000) * 1000
  return wallAsUTC - instantAtSecond
}

/** Minutes added to UTC to obtain local wall time at this instant. */
export function timezoneOffsetMinutes(
  timezone: Timezone,
  timestamp: number
): number {
  return timezoneOffsetMilliseconds(timezone, timestamp) / 60_000
}

function wallClockEpoch(wall: WallClock): number {
  const date = new Date(0)
  date.setUTCFullYear(wall.year, wall.month - 1, wall.day)
  date.setUTCHours(wall.hour, wall.minute, wall.second, wall.millisecond)
  return date.getTime()
}

function sameWallClock(left: WallClock, right: WallClock): boolean {
  return (
    left.year === right.year &&
    left.month === right.month &&
    left.day === right.day &&
    left.hour === right.hour &&
    left.minute === right.minute &&
    left.second === right.second &&
    left.millisecond === right.millisecond
  )
}

function instantsForWallClock(wall: WallClock, timezone: Timezone): number[] {
  const name = resolveTimezoneName(timezone)
  const naive = wallClockEpoch(wall)
  if (!Number.isFinite(naive)) return []

  const offsets = new Set<number>()
  for (const sample of [naive - 172_800_000, naive, naive + 172_800_000]) {
    offsets.add(timezoneOffsetMilliseconds(timezone, sample))
  }
  for (const offset of [...offsets]) {
    offsets.add(timezoneOffsetMilliseconds(timezone, naive - offset))
  }

  return [...offsets]
    .map(offset => naive - offset)
    .filter(timestamp => sameWallClock(wallClockAt(timestamp, name), wall))
    .filter((timestamp, index, values) => values.indexOf(timestamp) === index)
    .sort((a, b) => a - b)
}

function parseExplicitOffsetDateTime(
  text: string
): { matched: false } | { matched: true; timestamp: number | null } {
  const match = text.match(EXPLICIT_OFFSET_DATE_TIME)
  if (!match) return { matched: false }

  const [, year, month, day, hour, minute, second, fraction = '', zone] = match
  const wall: WallClock = {
    year: Number(year),
    month: Number(month),
    day: Number(day),
    hour: Number(hour),
    minute: Number(minute),
    second: Number(second),
    millisecond: Number(fraction.padEnd(3, '0')),
  }
  const naive = wallClockEpoch(wall)
  const utcWall = wallClockAt(naive, 'UTC')
  const offsetDigits =
    zone.toUpperCase() === 'Z' ? '' : zone.slice(1).replaceAll(':', '')
  const offsetParts = [
    offsetDigits.slice(0, 2) || '0',
    offsetDigits.slice(2, 4) || '0',
    offsetDigits.slice(4, 6) || '0',
  ].map(Number)
  if (
    !sameWallClock(utcWall, wall) ||
    offsetParts[0] > 23 ||
    offsetParts[1] > 59 ||
    offsetParts[2] > 59
  ) {
    return { matched: true, timestamp: null }
  }

  const offsetSeconds =
    zone.toUpperCase() === 'Z'
      ? 0
      : (zone.startsWith('-') ? -1 : 1) *
        (offsetParts[0] * 3600 + offsetParts[1] * 60 + offsetParts[2])
  return {
    matched: true,
    timestamp: naive - offsetSeconds * 1000,
  }
}

/** Parse natural-language input in the selected wall-clock timezone. */
export function parseDateTimeInTimezone(
  text: string,
  timezone: Timezone,
  now: number = Date.now(),
  disambiguation: WallClockDisambiguation = 'reject'
): ParsedDateTime {
  if (!text.trim()) return { success: false, error: 'Please enter a time' }

  const explicitOffset = parseExplicitOffsetDateTime(text.trim())
  if (explicitOffset.matched) {
    return explicitOffset.timestamp === null
      ? { success: false, error: 'Invalid time format' }
      : { success: true, timestamp: explicitOffset.timestamp }
  }

  try {
    const referenceOffset = timezoneOffsetMinutes(timezone, now)
    const parsed = chrono.parse(text, {
      instant: new Date(now),
      timezone: referenceOffset,
    })[0]
    if (!parsed) {
      return {
        success: false,
        error: 'Could not understand this time format',
      }
    }

    if (
      parsed.tags().has('result/relativeDateAndTime') ||
      parsed.start.isCertain('timezoneOffset')
    ) {
      return { success: true, timestamp: parsed.start.date().getTime() }
    }

    const component = (name: Parameters<typeof parsed.start.get>[0]) =>
      parsed.start.get(name) ?? 0
    const wall: WallClock = {
      year: component('year'),
      month: component('month'),
      day: component('day'),
      hour: component('hour'),
      minute: component('minute'),
      second: component('second'),
      millisecond: component('millisecond'),
    }
    const candidates = instantsForWallClock(wall, timezone)
    const name = resolveTimezoneName(timezone)
    if (candidates.length === 0) {
      return {
        success: false,
        error: `This time does not exist in ${name} because the clocks changed`,
      }
    }
    if (candidates.length > 1) {
      if (disambiguation === 'earlier') {
        return { success: true, timestamp: candidates[0] }
      }
      if (disambiguation === 'later') {
        return { success: true, timestamp: candidates[candidates.length - 1] }
      }
      return {
        success: false,
        error: `This time occurs twice in ${name}; include an explicit UTC offset`,
      }
    }
    return { success: true, timestamp: candidates[0] }
  } catch {
    return { success: false, error: 'Invalid time format' }
  }
}

/** ISO-like editable value with an explicit offset, so DST overlaps round-trip. */
export function formatEditableDateTime(
  timestamp: number,
  timezone: Timezone
): string {
  const wall = wallClockAt(timestamp, resolveTimezoneName(timezone))
  const offsetSeconds = Math.round(
    timezoneOffsetMilliseconds(timezone, timestamp) / 1000
  )
  const sign = offsetSeconds < 0 ? '-' : '+'
  const absoluteOffset = Math.abs(offsetSeconds)
  const offsetHours = Math.floor(absoluteOffset / 3600)
  const offsetMinutes = Math.floor((absoluteOffset % 3600) / 60)
  const offsetRemainderSeconds = absoluteOffset % 60
  const pad = (value: number, width = 2) => String(value).padStart(width, '0')
  const offset =
    offsetSeconds === 0
      ? 'Z'
      : `${sign}${pad(offsetHours)}:${pad(offsetMinutes)}${
          offsetRemainderSeconds ? `:${pad(offsetRemainderSeconds)}` : ''
        }`
  return `${pad(wall.year, 4)}-${pad(wall.month)}-${pad(wall.day)}T${pad(
    wall.hour
  )}:${pad(wall.minute)}:${pad(wall.second)}.${pad(wall.millisecond, 3)}${offset}`
}

/** Short timezone label for UI chrome (UTC, PST, …). */
export function formatTimezoneLabel(
  timezone: Timezone,
  date: Date = new Date()
): string {
  if (timezone === 'UTC') return 'UTC'
  return (
    new Intl.DateTimeFormat('en', {
      timeZone: resolveTimezoneName(timezone),
      timeZoneName: 'short',
    })
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

  return date.toLocaleString('en-CA', {
    ...options,
    timeZone: resolveTimezoneName(timezone),
  })
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
  const endTimezone = formatTimezoneLabel(timezone, new Date(end))
  const startLabel = formatDateTimeMs(start, timezone).dateTime
  const endLabel = formatDateTimeMs(end, timezone).dateTime
  const startTimezone = formatTimezoneLabel(timezone, new Date(start))
  if (includeTimezone && startTimezone !== endTimezone) {
    return `${startLabel} ${startTimezone} - ${endLabel} ${endTimezone}`
  }
  const range = `${startLabel} - ${endLabel}`
  return includeTimezone ? `${range} ${endTimezone}` : range
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
    const parsedRows = parsed as RecentTimeRange[]
    const rows = parsedRows.filter(
      row =>
        row !== null &&
        typeof row === 'object' &&
        Number.isFinite(row.start) &&
        Number.isFinite(row.end) &&
        Number.isFinite(row.usedAt) &&
        row.start < row.end
    )
    const sorted = [...rows].sort((a, b) => b.usedAt - a.usedAt)
    const trimmed = sorted.slice(0, MAX_RECENT_TIME_RANGES)
    if (trimmed.length < parsedRows.length) {
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
  if (!Number.isFinite(start) || !Number.isFinite(end) || start >= end) return
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

/** Remove one exact range and return the remaining recents. */
export function removeRecentTimeRange(
  start: number,
  end: number
): RecentTimeRange[] {
  const recentTimeRanges = loadRecentTimeRanges().filter(
    range => range.start !== start || range.end !== end
  )
  if (recentTimeRanges.length === 0) {
    localStorage.removeItem(RECENT_STORAGE_KEY)
    return []
  }
  localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(recentTimeRanges))
  return recentTimeRanges
}
