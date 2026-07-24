import type { Timezone } from '@/utils/time'

export type ChartTimeRangeLabels = {
  start: string
  /** Present only when end falls on a different calendar day than start. */
  end?: string
}

function calendarDayKey(ms: number, timezone: Timezone): string {
  const opts: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour12: false,
  }
  if (timezone === 'UTC') opts.timeZone = 'UTC'
  return new Intl.DateTimeFormat('en-CA', opts).format(new Date(ms))
}

function formatChartAxisDate(ms: number, timezone: Timezone): string {
  const opts: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  }
  if (timezone === 'UTC') opts.timeZone = 'UTC'
  return new Intl.DateTimeFormat('en', opts).format(new Date(ms))
}

/** Time-of-day labels for chart x-axis ticks (date lives in the range header). */
export function formatChartAxisTime(
  value: Date | number,
  timezone: Timezone
): string {
  const ms = value instanceof Date ? value.getTime() : value
  const opts: Intl.DateTimeFormatOptions = {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }
  if (timezone === 'UTC') opts.timeZone = 'UTC'
  return new Intl.DateTimeFormat('en', opts).format(new Date(ms))
}

/** Date-only labels for the range strip above a chart. */
export function getChartTimeRangeLabels(
  startMs: number,
  endMs: number,
  timezone: Timezone
): ChartTimeRangeLabels {
  const start = formatChartAxisDate(startMs, timezone)
  if (calendarDayKey(startMs, timezone) === calendarDayKey(endMs, timezone)) {
    return { start }
  }
  return { start, end: formatChartAxisDate(endMs, timezone) }
}
