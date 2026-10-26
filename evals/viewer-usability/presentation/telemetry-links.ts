const object = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}

const timestamp = (value: unknown): string | null => {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/.test(value)) return null
  const date = new Date(value)
  return Number.isFinite(date.getTime()) && date.toISOString() === value ? value : null
}

// Only identifiers, timestamps and an explicitly selected viewer origin cross into the
// report. Never pass commands, error payloads, evidence paths or arbitrary URLs.
export function telemetryLinks(value: unknown, manifest: Record<string, unknown>) {
  const identity = object(value)
  const traceID = identity.traceID
  const spanID = identity.spanID
  if (typeof traceID !== 'string' || !/^[a-f0-9]{32}$/.test(traceID) || /^0+$/.test(traceID)) return null
  const supplied = object(manifest.telemetryViewers)[traceID] ?? manifest.telemetryViewer
  if (typeof supplied !== 'string') return null
  let origin = ''
  if (supplied !== '') {
    try {
      const url = new URL(supplied)
      if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.pathname !== '/' || url.search || url.hash) return null
      origin = url.origin
    } catch { return null }
  }
  const trace = `${origin}/traces/${traceID}`
  const validSpan = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{16}$/.test(value) && !/^0+$/.test(value)
  const indexedTools = object(manifest.telemetryTools)[traceID]
  const indexedTimes = new Map(Array.isArray(indexedTools)
    ? indexedTools.map(object).map(tool => [tool.spanID, timestamp(tool.startedAt)] as const) : [])
  const recordedTools = identity.tools ?? indexedTools
  const tools = Array.isArray(recordedTools) ? recordedTools.map(object).filter(tool => validSpan(tool.spanID)).map((tool, index) => ({
    traceID,
    spanID: String(tool.spanID),
    href: `${trace}?span=${tool.spanID}`,
    name: typeof tool.name === 'string' && /^[a-zA-Z0-9_. -]{1,60}$/.test(tool.name) ? tool.name.replace(/^tool /, '') : 'tool',
    number: index + 1,
    startedAt: timestamp(tool.startedAt) ?? indexedTimes.get(tool.spanID) ?? null,
    error: tool.error === true,
  })) : []
  return { traceID, spanID: validSpan(spanID) ? spanID : null, trace, span: validSpan(spanID) ? `${trace}?span=${spanID}` : null, tools }
}
