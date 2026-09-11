export const VIEW_SPEC_VERSION = 1 as const

export const TRACE_SORT_FIELDS = [
  'serviceName',
  'rootSpanName',
  'startTime',
  'duration',
  'spanCount',
  'errorCount',
] as const

export const LOG_SORT_FIELDS = [
  'timestamp',
  'severity',
  'service',
  'body',
] as const

export const METRIC_SORT_FIELDS = [
  'name',
  'metricType',
  'serviceName',
  'description',
  'dataPointCount',
  'seriesCount',
  'lastSeen',
] as const

export const HISTOGRAM_QUANTILE_KEYS = ['0.5', '0.95', '0.99'] as const

export type SortDirection = 'asc' | 'desc'
export type TraceSortField = (typeof TRACE_SORT_FIELDS)[number]
export type LogSortField = (typeof LOG_SORT_FIELDS)[number]
export type MetricSortField = (typeof METRIC_SORT_FIELDS)[number]
export type HistogramQuantileKey = (typeof HISTOGRAM_QUANTILE_KEYS)[number]

export type ViewTimeV1 =
  { kind: 'all' } | { kind: 'absolute'; startMs: number; endMs: number }

export type SignalListViewV1<TField extends string> = {
  search: string
  sort: {
    field: TField
    direction: SortDirection
  }
}

export type MetricQueryTimezoneV1 =
  { kind: 'iana'; name: string } | { kind: 'offset'; utcOffsetMinutes: number }

export type TimeseriesMetricDetailV1 = {
  kind: 'timeseries'
  aggregation: 'raw' | 'sum' | 'avg' | 'rate'
  queryTimezone: MetricQueryTimezoneV1
  selectedSeries: string | null
  selectedDatapoint: string | null
  visibleSeries: string[]
  showAllSeriesAggregate: boolean
  showSelectionStatOverlays: boolean
}

export type HistogramMetricDetailV1 = {
  kind: 'histogram'
  tab: 'heatmap' | 'quantiles' | 'histogram'
  scope: 'window' | 'bucket'
  queryTimezone: MetricQueryTimezoneV1
  selectedSeries: string | null
  selectedDatapoint: string | null
  visibleSeries: string[]
  activeQuantile: HistogramQuantileKey
}

export type MetricDetailV1 = TimeseriesMetricDetailV1 | HistogramMetricDetailV1

export type HomeViewSpecV1 = {
  version: typeof VIEW_SPEC_VERSION
  time: ViewTimeV1
  destination: { signal: 'home' }
}

export type TraceViewSpecV1 = {
  version: typeof VIEW_SPEC_VERSION
  time: ViewTimeV1
  destination: {
    signal: 'traces'
    traceID: string | null
    spanID: string | null
    eventIndex: number | null
  }
  list: SignalListViewV1<TraceSortField>
}

export type LogViewSpecV1 = {
  version: typeof VIEW_SPEC_VERSION
  time: ViewTimeV1
  destination: {
    signal: 'logs'
    logID: string | null
  }
  list: SignalListViewV1<LogSortField>
}

export type MetricViewSpecV1 = {
  version: typeof VIEW_SPEC_VERSION
  time: ViewTimeV1
  destination: {
    signal: 'metrics'
    metricID: string | null
    detail: MetricDetailV1 | null
  }
  list: SignalListViewV1<MetricSortField>
}

export type ViewSpecV1 =
  HomeViewSpecV1 | TraceViewSpecV1 | LogViewSpecV1 | MetricViewSpecV1

type UnknownRecord = Record<string, unknown>

const SORT_DIRECTIONS = ['asc', 'desc'] as const
const AGGREGATIONS = ['raw', 'sum', 'avg', 'rate'] as const
const HISTOGRAM_TABS = ['heatmap', 'quantiles', 'histogram'] as const
const HISTOGRAM_SCOPES = ['window', 'bucket'] as const
const MAX_SEARCH_BYTES = 65_536
const MAX_TIMESERIES_VISIBLE = 22
const MAX_HISTOGRAM_VISIBLE = 10_000
const MAX_CANONICAL_BYTES = 524_288
const MAX_TIMEZONE_NAME_BYTES = 255
const MAX_TIMESTAMP_MS = 9_223_372_036_854
const TRACE_ID = /^(?!0{32}$)[0-9a-f]{32}$/i
const SPAN_ID = /^(?!0{16}$)[0-9a-f]{16}$/i
const UUID =
  /^(?!00000000-0000-0000-0000-000000000000$)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

function record(value: unknown): UnknownRecord | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as UnknownRecord)
    : null
}

function member<T extends string>(
  value: unknown,
  values: readonly T[]
): T | null {
  return typeof value === 'string' && values.includes(value as T)
    ? (value as T)
    : null
}

function hasCanonicalUnicode(value: string): boolean {
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i)
    if (code === 0x2028 || code === 0x2029) return false
    if (code >= 0xd800 && code <= 0xdbff) {
      if (i + 1 >= value.length) return false
      const next = value.charCodeAt(++i)
      if (next < 0xdc00 || next > 0xdfff) return false
    } else if (code >= 0xdc00 && code <= 0xdfff) {
      return false
    }
  }
  return true
}

function utf8Length(value: string): number {
  return new TextEncoder().encode(value).length
}

function identifier(
  value: unknown,
  pattern: RegExp
): string | null | undefined {
  if (value === null) return null
  return typeof value === 'string' && pattern.test(value)
    ? value.toLowerCase()
    : undefined
}

function boolean(value: unknown): boolean | null {
  return typeof value === 'boolean' ? value : null
}

function compareCodeUnits(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

function identifierSet(
  value: unknown,
  pattern: RegExp,
  maxItems: number
): string[] | null {
  if (!Array.isArray(value)) return null

  const normalized = new Set<string>()
  for (const item of value) {
    const parsed = identifier(item, pattern)
    if (typeof parsed !== 'string') return null
    normalized.add(parsed)
    if (normalized.size > maxItems) return null
  }
  return [...normalized].sort(compareCodeUnits)
}

function parseMetricQueryTimezone(
  value: unknown
): MetricQueryTimezoneV1 | null {
  const candidate = record(value)
  if (candidate?.kind === 'offset') {
    const utcOffsetMinutes = candidate.utcOffsetMinutes
    return Number.isInteger(utcOffsetMinutes) &&
      (utcOffsetMinutes as number) >= -14 * 60 &&
      (utcOffsetMinutes as number) <= 14 * 60
      ? { kind: 'offset', utcOffsetMinutes: utcOffsetMinutes as number }
      : null
  }
  if (candidate?.kind !== 'iana' || typeof candidate.name !== 'string') {
    return null
  }
  const name = candidate.name
  if (
    name.length === 0 ||
    utf8Length(name) > MAX_TIMEZONE_NAME_BYTES ||
    !hasCanonicalUnicode(name) ||
    !/^[A-Za-z][A-Za-z0-9._+-]*(?:\/[A-Za-z0-9][A-Za-z0-9._+-]*)*$/.test(name)
  ) {
    return null
  }
  return { kind: 'iana', name }
}

function parseTime(value: unknown): ViewTimeV1 | null {
  const candidate = record(value)
  if (!candidate) return null
  if (candidate.kind === 'all') return { kind: 'all' }
  if (candidate.kind !== 'absolute') return null

  const { startMs, endMs } = candidate
  if (
    !Number.isSafeInteger(startMs) ||
    !Number.isSafeInteger(endMs) ||
    (startMs as number) < 0 ||
    (endMs as number) > MAX_TIMESTAMP_MS ||
    (endMs as number) <= (startMs as number)
  ) {
    return null
  }
  return {
    kind: 'absolute',
    startMs: startMs as number,
    endMs: endMs as number,
  }
}

function parseList<TField extends string>(
  value: unknown,
  fields: readonly TField[]
): SignalListViewV1<TField> | null {
  const candidate = record(value)
  const sort = record(candidate?.sort)
  const field = member(sort?.field, fields)
  const direction = member(sort?.direction, SORT_DIRECTIONS)
  if (
    !candidate ||
    typeof candidate.search !== 'string' ||
    utf8Length(candidate.search) > MAX_SEARCH_BYTES ||
    !hasCanonicalUnicode(candidate.search) ||
    !field ||
    !direction
  ) {
    return null
  }
  return {
    search: candidate.search,
    sort: { field, direction },
  }
}

function parseMetricDetail(value: unknown): MetricDetailV1 | null {
  const candidate = record(value)
  if (!candidate) return null

  const queryTimezone = parseMetricQueryTimezone(candidate.queryTimezone)
  const selectedSeries = identifier(candidate.selectedSeries, UUID)
  const selectedDatapoint = identifier(candidate.selectedDatapoint, UUID)
  if (
    !queryTimezone ||
    selectedSeries === undefined ||
    selectedDatapoint === undefined ||
    (selectedDatapoint !== null && selectedSeries === null)
  ) {
    return null
  }

  if (candidate.kind === 'timeseries') {
    const aggregation = member(candidate.aggregation, AGGREGATIONS)
    const visibleSeries = identifierSet(
      candidate.visibleSeries,
      UUID,
      MAX_TIMESERIES_VISIBLE
    )
    const showAllSeriesAggregate = boolean(candidate.showAllSeriesAggregate)
    const showSelectionStatOverlays = boolean(
      candidate.showSelectionStatOverlays
    )
    if (
      !aggregation ||
      !visibleSeries ||
      showAllSeriesAggregate === null ||
      showSelectionStatOverlays === null ||
      (selectedSeries !== null && !visibleSeries.includes(selectedSeries))
    ) {
      return null
    }
    return {
      kind: 'timeseries',
      aggregation,
      queryTimezone,
      selectedSeries,
      selectedDatapoint,
      visibleSeries,
      showAllSeriesAggregate,
      showSelectionStatOverlays,
    }
  }

  if (candidate.kind === 'histogram') {
    const tab = member(candidate.tab, HISTOGRAM_TABS)
    const scope = member(candidate.scope, HISTOGRAM_SCOPES)
    const visibleSeries = identifierSet(
      candidate.visibleSeries,
      UUID,
      MAX_HISTOGRAM_VISIBLE
    )
    const activeQuantile = member(
      candidate.activeQuantile,
      HISTOGRAM_QUANTILE_KEYS
    )
    if (
      !tab ||
      !scope ||
      !visibleSeries ||
      !activeQuantile ||
      (selectedSeries !== null && !visibleSeries.includes(selectedSeries))
    ) {
      return null
    }
    return {
      kind: 'histogram',
      tab,
      scope,
      queryTimezone,
      selectedSeries,
      selectedDatapoint,
      visibleSeries,
      activeQuantile,
    }
  }

  return null
}

function parseTraceSpec(
  time: ViewTimeV1,
  destination: UnknownRecord,
  value: UnknownRecord
): TraceViewSpecV1 | null {
  const traceID = identifier(destination.traceID, TRACE_ID)
  const spanID = identifier(destination.spanID, SPAN_ID)
  const eventIndex = destination.eventIndex
  const list = parseList(value.list, TRACE_SORT_FIELDS)
  if (
    traceID === undefined ||
    spanID === undefined ||
    !list ||
    (eventIndex !== null &&
      (!Number.isSafeInteger(eventIndex) || (eventIndex as number) < 0)) ||
    (traceID === null && (spanID !== null || eventIndex !== null)) ||
    (spanID === null && eventIndex !== null)
  ) {
    return null
  }
  return {
    version: VIEW_SPEC_VERSION,
    time,
    destination: {
      signal: 'traces',
      traceID,
      spanID,
      eventIndex: eventIndex as number | null,
    },
    list,
  }
}

function parseLogSpec(
  time: ViewTimeV1,
  destination: UnknownRecord,
  value: UnknownRecord
): LogViewSpecV1 | null {
  const logID = identifier(destination.logID, UUID)
  const list = parseList(value.list, LOG_SORT_FIELDS)
  if (logID === undefined || !list) return null
  return {
    version: VIEW_SPEC_VERSION,
    time,
    destination: { signal: 'logs', logID },
    list,
  }
}

function parseMetricSpec(
  time: ViewTimeV1,
  destination: UnknownRecord,
  value: UnknownRecord
): MetricViewSpecV1 | null {
  const metricID = identifier(destination.metricID, UUID)
  let detail: MetricDetailV1 | null
  if (destination.detail === null) {
    detail = null
  } else {
    detail = parseMetricDetail(destination.detail)
    if (!detail) return null
  }
  const list = parseList(value.list, METRIC_SORT_FIELDS)
  if (
    metricID === undefined ||
    !list ||
    (metricID === null && detail !== null) ||
    (metricID !== null && detail === null)
  ) {
    return null
  }
  return {
    version: VIEW_SPEC_VERSION,
    time,
    destination: { signal: 'metrics', metricID, detail },
    list,
  }
}

function normalizeViewSpecV1Unchecked(value: unknown): ViewSpecV1 | null {
  const candidate = record(value)
  const destination = record(candidate?.destination)
  if (!candidate || candidate.version !== VIEW_SPEC_VERSION || !destination) {
    return null
  }

  const time = parseTime(candidate.time)
  if (!time) return null

  switch (destination.signal) {
    case 'home':
      return {
        version: VIEW_SPEC_VERSION,
        time,
        destination: { signal: 'home' },
      }
    case 'traces':
      return parseTraceSpec(time, destination, candidate)
    case 'logs':
      return parseLogSpec(time, destination, candidate)
    case 'metrics':
      return parseMetricSpec(time, destination, candidate)
    default:
      return null
  }
}

/** Validates and normalizes an untrusted V1 view snapshot. */
export function normalizeViewSpecV1(value: unknown): ViewSpecV1 | null {
  const normalized = normalizeViewSpecV1Unchecked(value)
  if (!normalized) return null
  return utf8Length(JSON.stringify(normalized)) <= MAX_CANONICAL_BYTES
    ? normalized
    : null
}

/** Parses and validates canonical or non-canonical JSON input. */
export function parseViewSpecV1JSON(json: string): ViewSpecV1 | null {
  if (utf8Length(json) > MAX_CANONICAL_BYTES) return null
  try {
    return normalizeViewSpecV1(JSON.parse(json))
  } catch {
    return null
  }
}

/** Returns the one stable JSON representation used for revision identity. */
export function canonicalViewSpecV1JSON(value: unknown): string {
  const normalized = normalizeViewSpecV1(value)
  if (!normalized) throw new TypeError('Invalid ViewSpecV1')
  return JSON.stringify(normalized)
}

function base64URL(bytes: Uint8Array): string {
  const alphabet =
    'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
  let encoded = ''
  for (let i = 0; i < bytes.length; i += 3) {
    const a = bytes[i] ?? 0
    const b = bytes[i + 1]
    const c = bytes[i + 2]
    encoded += alphabet[a >> 2]
    encoded += alphabet[((a & 0x03) << 4) | ((b ?? 0) >> 4)]
    if (b !== undefined) {
      encoded += alphabet[((b & 0x0f) << 2) | ((c ?? 0) >> 6)]
    }
    if (c !== undefined) encoded += alphabet[c & 0x3f]
  }
  return encoded
}

/** Computes the immutable, URL-safe identity of a canonical V1 view snapshot. */
export async function viewSpecRevisionID(value: unknown): Promise<string> {
  const canonical = canonicalViewSpecV1JSON(value)
  if (!globalThis.crypto?.subtle) {
    throw new Error('Web Crypto is unavailable')
  }
  const digest = await globalThis.crypto.subtle.digest(
    'SHA-256',
    new TextEncoder().encode(canonical)
  )
  return `v${VIEW_SPEC_VERSION}_${base64URL(new Uint8Array(digest))}`
}
