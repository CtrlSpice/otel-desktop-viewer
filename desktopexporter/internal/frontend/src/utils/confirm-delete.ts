/** Ask before changing stored telemetry, including records hidden by filters. */
export function confirmDeleteAll(signal: 'trace' | 'metric' | 'log'): boolean {
  return window.confirm(
    `Are you sure you want to delete ALL stored ${signal}s, including those outside the current filters?\n\nThis cannot be undone.`
  )
}
