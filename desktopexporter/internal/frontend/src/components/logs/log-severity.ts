// Severity band logic for OpenTelemetry log severityNumber values.
// Lifted out of LogsPage.svelte so SignalBadges and LogDetailView can
// import severity helpers without reaching into a page module.

export type SeverityBand =
  | 'trace'
  | 'debug'
  | 'info'
  | 'warn'
  | 'error'
  | 'fatal'

export function severityBand(severityNumber: number): SeverityBand {
  if (severityNumber <= 4) return 'trace'
  if (severityNumber <= 8) return 'debug'
  if (severityNumber <= 12) return 'info'
  if (severityNumber <= 16) return 'warn'
  if (severityNumber <= 20) return 'error'
  return 'fatal'
}

const BADGE_CLASS: Record<SeverityBand, string> = {
  trace: 'badge badge-xs badge-soft badge-neutral',
  debug: 'badge badge-xs badge-soft badge-success',
  info: 'badge badge-xs badge-soft badge-info',
  warn: 'badge badge-xs badge-soft badge-warning',
  error: 'badge badge-xs badge-soft badge-error',
  fatal: 'badge badge-xs badge-soft badge-error border border-error/50',
}

const BORDER_CLASS: Record<SeverityBand, string> = {
  trace: 'border-l-neutral/40',
  debug: 'border-l-success/40',
  info: 'border-l-info/40',
  warn: 'border-l-warning/40',
  error: 'border-l-error/40',
  fatal: 'border-l-error',
}

export function severityBadgeClass(severityNumber: number): string {
  return BADGE_CLASS[severityBand(severityNumber)]
}

export function severityBorderClass(severityNumber: number): string {
  return BORDER_CLASS[severityBand(severityNumber)]
}

export function severityLabel(
  severityText: string,
  severityNumber: number
): string {
  return severityText || severityBand(severityNumber).toUpperCase()
}
