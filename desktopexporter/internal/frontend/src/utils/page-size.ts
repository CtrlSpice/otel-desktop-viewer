export const PAGE_SIZES = [25, 50, 100] as const
export type PageSize = (typeof PAGE_SIZES)[number]

export function isPageSize(value: number): value is PageSize {
  return PAGE_SIZES.some(size => size === value)
}

export function loadPageSize(key: string): PageSize {
  try {
    const value = Number(localStorage.getItem(key))
    return isPageSize(value) ? value : 25
  } catch {
    return 25
  }
}

export function savePageSize(key: string, value: PageSize): void {
  try {
    localStorage.setItem(key, String(value))
  } catch {
    // The current page still works when storage is blocked.
  }
}
