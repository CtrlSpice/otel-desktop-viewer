/** Preserve a visible chart endpoint when pane resizing introduces overflow.
 * An intentional scroll away from the end retains its own position. */
export function preserveChartScrollEnd(viewport: HTMLElement) {
  let width = viewport.clientWidth
  let contentWidth = viewport.scrollWidth
  let left = viewport.scrollLeft

  const onScroll = () => {
    left = viewport.scrollLeft
  }
  const observer = new ResizeObserver(() => {
    const nextWidth = viewport.clientWidth
    const nextContentWidth = viewport.scrollWidth
    const wasAtEnd = contentWidth - width - left <= 1
    const resized = nextWidth !== width
    width = nextWidth
    contentWidth = nextContentWidth
    if (resized && wasAtEnd)
      viewport.scrollLeft = Math.max(0, contentWidth - width)
    left = viewport.scrollLeft
  })
  observer.observe(viewport)
  if (viewport.firstElementChild) observer.observe(viewport.firstElementChild)
  viewport.addEventListener('scroll', onScroll, { passive: true })

  return {
    destroy() {
      observer.disconnect()
      viewport.removeEventListener('scroll', onScroll)
    },
  }
}
