export type HistogramViewPreference = 'heatmap' | 'histogram'

const STORAGE_KEY = 'metrics:histogram-view'

/** Histogram and exponential histogram share the last explicitly chosen tab. */
export function loadHistogramViewPreference(): HistogramViewPreference {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'histogram'
      ? 'histogram'
      : 'heatmap'
  } catch {
    return 'heatmap'
  }
}

export function saveHistogramViewPreference(
  view: HistogramViewPreference
): void {
  try {
    localStorage.setItem(STORAGE_KEY, view)
  } catch {
    // Storage availability must not prevent switching chart tabs.
  }
}
