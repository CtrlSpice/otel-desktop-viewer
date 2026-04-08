const STORAGE_KEY = 'otel-desktop-viewer:trace-list-col-widths-v6'

/**
 * Resizable trace list columns (px). Checkbox, root indicator, and count cols use fixed constants in TracesPage.
 */
export type TraceListColWidths = {
  traceId: number
  rootName: number
  service: number
  startTime: number
  duration: number
}

export const MIN_WIDTHS: TraceListColWidths = {
  traceId: 140,
  rootName: 80,
  service: 100,
  startTime: 140,
  duration: 60,
}

const DEFAULT_WIDTHS: TraceListColWidths = {
  traceId: 260,
  rootName: 220,
  service: 160,
  startTime: 180,
  duration: 80,
}

export function loadTraceListColWidths(): TraceListColWidths {
  if (typeof localStorage === 'undefined') return { ...DEFAULT_WIDTHS }
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return { ...DEFAULT_WIDTHS }
    const o = JSON.parse(raw) as Partial<TraceListColWidths>
    return {
      traceId: clamp(o.traceId, MIN_WIDTHS.traceId, DEFAULT_WIDTHS.traceId),
      rootName: clamp(o.rootName, MIN_WIDTHS.rootName, DEFAULT_WIDTHS.rootName),
      service: clamp(o.service, MIN_WIDTHS.service, DEFAULT_WIDTHS.service),
      startTime: clamp(o.startTime, MIN_WIDTHS.startTime, DEFAULT_WIDTHS.startTime),
      duration: clamp(o.duration, MIN_WIDTHS.duration, DEFAULT_WIDTHS.duration),
    }
  } catch {
    return { ...DEFAULT_WIDTHS }
  }
}

export function saveTraceListColWidths(widths: TraceListColWidths): void {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(STORAGE_KEY, JSON.stringify(widths))
}

function clamp(
  value: number | undefined,
  min: number,
  fallback: number
): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return Math.max(min, Math.round(value))
}
