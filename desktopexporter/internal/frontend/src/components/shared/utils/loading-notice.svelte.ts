export const LOADING_NOTICE_DELAY_MS = 200
export type LoadingRequestKey = string | number | AbortSignal | null | undefined

/** Delay feedback, never the request or the replacement data. */
export function createLoadingNotice(
  pending: () => boolean,
  requestKey: () => LoadingRequestKey = () => undefined
) {
  let visible = $state(false)
  $effect.pre(() => {
    void requestKey()
    visible = false
    if (!pending()) return
    const timer = setTimeout(() => (visible = true), LOADING_NOTICE_DELAY_MS)
    return () => clearTimeout(timer)
  })
  return {
    get visible() {
      return pending() && visible
    },
  }
}
