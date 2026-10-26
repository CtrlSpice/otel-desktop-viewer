import type { Action } from 'svelte/action'

const LABEL_GAP = 12
const X_LABEL_GAP = 4

/** Reserve room for values beside the Y-axis and the first angled X-axis label. */
export function chartAxisGutter(
  yLabelWidth: number,
  xLabelWidth: number
): number {
  return Math.ceil(Math.max(yLabelWidth + LABEL_GAP, xLabelWidth + X_LABEL_GAP))
}

/** Measure the rendered font, not a character-count estimate or a fixed inset. */
export const measureChartAxisGutter: Action<
  HTMLElement,
  (left: number) => void
> = (element, report) => {
  let frame: number | undefined
  let previous: number | undefined
  let disposed = false
  const observed = new Set<SVGElement>()
  const resize = new ResizeObserver(schedule)

  function measure() {
    frame = undefined
    if (disposed) return
    const axes = new Set(element.querySelectorAll<SVGElement>('.lc-axis'))
    for (const axis of observed) {
      if (!axes.has(axis)) {
        resize.unobserve(axis)
        observed.delete(axis)
      }
    }
    for (const axis of axes) {
      if (!observed.has(axis)) {
        observed.add(axis)
        resize.observe(axis)
      }
    }
    const yLabels = element.querySelectorAll<SVGTextElement>(
      '.placement-left .lc-axis-tick-label'
    )
    if (!yLabels.length) return
    let yWidth = 0
    let xWidth = 0
    for (const label of yLabels)
      yWidth = Math.max(yWidth, label.getBoundingClientRect().width)
    for (const label of element.querySelectorAll<SVGTextElement>(
      '.placement-bottom .lc-axis-tick-label'
    )) {
      xWidth = Math.max(xWidth, label.getBoundingClientRect().width)
    }
    // Hidden/unmeasured charts retain their fallback until they can be measured.
    if (yWidth === 0) return
    const left = chartAxisGutter(yWidth, xWidth)
    if (left !== previous) {
      previous = left
      report(left)
    }
  }

  function schedule() {
    if (!disposed && frame === undefined) frame = requestAnimationFrame(measure)
  }

  const mutations = new MutationObserver(records => {
    if (
      records.some(record => {
        const target =
          record.target instanceof Element
            ? record.target
            : record.target.parentElement
        return target?.closest('.lc-axis') || record.type === 'childList'
      })
    )
      schedule()
  })
  mutations.observe(element, {
    childList: true,
    characterData: true,
    subtree: true,
  })
  resize.observe(element)
  document.fonts?.addEventListener('loadingdone', schedule)
  void document.fonts?.ready.then(schedule)
  schedule()

  return {
    update(next) {
      report = next
      previous = undefined
      schedule()
    },
    destroy() {
      disposed = true
      if (frame !== undefined) cancelAnimationFrame(frame)
      resize.disconnect()
      mutations.disconnect()
      document.fonts?.removeEventListener('loadingdone', schedule)
    },
  }
}
