export type HistogramViewPreference = 'heatmap' | 'histogram'

const STORAGE_KEY = 'metrics:histogram-view'
const PERCENTILES_STORAGE_KEY = 'metrics:histogram-percentiles'

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

export function loadHistogramPercentilesPreference(): boolean {
  try {
    return localStorage.getItem(PERCENTILES_STORAGE_KEY) !== 'false'
  } catch {
    return true
  }
}

export function saveHistogramPercentilesPreference(visible: boolean): void {
  try {
    localStorage.setItem(PERCENTILES_STORAGE_KEY, String(visible))
  } catch {
    // Storage availability must not prevent toggling percentiles.
  }
}
