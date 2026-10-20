/** Keep row hover continuous across a full-height column resize hit strip. */
export function resizeRowHover(handle: HTMLElement) {
  const container = handle.parentElement
  let hovered: Element | null = null
  let pointer: { x: number; y: number } | null = null

  function show(row: Element | null) {
    if (row === hovered) return
    hovered?.classList.remove('table-row--resize-hover')
    hovered = row
    hovered?.classList.add('table-row--resize-hover')
  }

  function update() {
    if (!pointer) return
    const elements = handle.ownerDocument.elementsFromPoint(pointer.x, pointer.y)
    if (!handle.contains(elements[0] ?? null)) {
      show(null)
      return
    }
    const row = elements
      .map(element => element.closest('.table-row'))
      .find(row => row !== null && container?.contains(row))
    show(row ?? null)
  }

  function move(event: PointerEvent) {
    pointer = { x: event.clientX, y: event.clientY }
    update()
  }

  function clear() {
    pointer = null
    show(null)
  }

  handle.addEventListener('pointerenter', move)
  handle.addEventListener('pointermove', move)
  handle.addEventListener('pointerleave', clear)
  handle.addEventListener('pointercancel', clear)
  container?.addEventListener('scroll', update, true)
  return {
    destroy() {
      clear()
      handle.removeEventListener('pointerenter', move)
      handle.removeEventListener('pointermove', move)
      handle.removeEventListener('pointerleave', clear)
      handle.removeEventListener('pointercancel', clear)
      container?.removeEventListener('scroll', update, true)
    },
  }
}
