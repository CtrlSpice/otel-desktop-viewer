/**
 * The mechanics every drag handle needs, and nothing about what a drag means.
 *
 * @remarks
 * Three handles in this app resize things -- the panel split, the waterfall's
 * columns, the signal list drawer -- and each had grown its own version of
 * this, complete in different places. The panel split suppressed text
 * selection; the columns did not, so dragging one selected span names as it
 * went and the cursor flipped to an I-beam over every label it crossed. None
 * opted out of touch scrolling.
 *
 * What is shared here is the *input device*: a pointer is down and moving.
 * What each caller keeps is the meaning -- a fraction of a container, pixels
 * across weighted columns, one element's width -- along with its own clamping
 * and persistence. A helper that also took those would need a mode flag per
 * caller, which is three implementations in one file rather than three files,
 * and then a change to one caller's clamping can break another's.
 *
 * So the interface is a pixel delta. Pixels are what a pointer produces;
 * everything past that is the caller's business.
 */

export type DragOptions = {
  /** Which axis the delta is measured along. */
  axis: 'x' | 'y'
  /** Pixels moved since the drag began, signed. */
  onMove: (delta: number) => void
  /** Fires once, on pointerup, pointercancel, or cancel(). */
  onEnd?: () => void
}

/** Ends a drag early, for a component unmounting mid-gesture. */
export type DragHandle = { cancel: () => void }

export function startDrag(e: PointerEvent, opts: DragOptions): DragHandle {
  // Stops the browser starting a text selection or a native drag from the
  // same press. Without it the first pointermove selects whatever the handle
  // happens to sit on.
  e.preventDefault()

  const start = opts.axis === 'x' ? e.clientX : e.clientY
  const target = e.currentTarget as HTMLElement | null

  // Capture routes every later pointer event to this element, so a fast drag
  // that outruns the cursor -- or leaves the window -- keeps resizing instead
  // of stopping wherever the pointer escaped. Window listeners below are the
  // backstop for the browsers and edge cases where capture is refused.
  try {
    target?.setPointerCapture(e.pointerId)
  } catch {
    /* window listeners still end the drag */
  }

  // The cursor is set on the body, not the handle: mid-drag the pointer is
  // usually over some other element, and that element's cursor would win.
  // Same for selection -- suppressing it on the handle does nothing once the
  // pointer is over the text being selected.
  const prevCursor = document.body.style.cursor
  const prevSelect = document.body.style.userSelect
  document.body.style.cursor = opts.axis === 'x' ? 'col-resize' : 'row-resize'
  document.body.style.userSelect = 'none'

  let done = false

  function move(ev: PointerEvent) {
    if (done) return
    opts.onMove((opts.axis === 'x' ? ev.clientX : ev.clientY) - start)
  }

  function end() {
    if (done) return
    done = true
    document.body.style.cursor = prevCursor
    document.body.style.userSelect = prevSelect
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', end)
    window.removeEventListener('pointercancel', end)
    try {
      if (target?.hasPointerCapture(e.pointerId)) {
        target.releasePointerCapture(e.pointerId)
      }
    } catch {
      /* already released */
    }
    opts.onEnd?.()
  }

  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', end)
  window.addEventListener('pointercancel', end)

  return { cancel: end }
}
