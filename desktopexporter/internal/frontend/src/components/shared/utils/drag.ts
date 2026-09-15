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
  /** Fires once when the active pointer ends, capture is lost, focus leaves, or cancel() runs. */
  onEnd?: () => void
}

/** Ends a drag early, for a component unmounting mid-gesture. */
export type DragHandle = { cancel: () => void }

// Body cursor and selection are shared document state, so only one helper drag
// may own them at a time in each document.
const activeDrags = new WeakMap<Document, DragHandle>()
const replacingDrags = new WeakSet<Document>()

type DragStartEvent = Pick<
  PointerEvent,
  'clientX' | 'clientY' | 'currentTarget' | 'pointerId' | 'preventDefault'
>

export function startDrag(e: DragStartEvent, opts: DragOptions): DragHandle {
  // Stops the browser starting a text selection or a native drag from the
  // same press. Without it the first pointermove selects whatever the handle
  // happens to sit on.
  e.preventDefault()

  const start = opts.axis === 'x' ? e.clientX : e.clientY
  const target = e.currentTarget as HTMLElement | null
  const ownerDocument = target?.ownerDocument ?? document
  const ownerWindow = ownerDocument.defaultView ?? window

  // The initiating call owns this replacement. An onEnd callback may try to
  // start another drag synchronously; reject that reentrant start rather than
  // allowing callbacks to form an unbounded cancellation loop.
  if (replacingDrags.has(ownerDocument)) {
    return { cancel() {} }
  }
  const activeDrag = activeDrags.get(ownerDocument)
  if (activeDrag) {
    replacingDrags.add(ownerDocument)
    try {
      activeDrag.cancel()
    } finally {
      replacingDrags.delete(ownerDocument)
    }
  }

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
  const prevCursor = ownerDocument.body.style.cursor
  const prevSelect = ownerDocument.body.style.userSelect
  ownerDocument.body.style.cursor =
    opts.axis === 'x' ? 'col-resize' : 'row-resize'
  ownerDocument.body.style.userSelect = 'none'

  let done = false

  function isActivePointer(ev: PointerEvent) {
    return ev.pointerId === e.pointerId
  }

  function move(ev: PointerEvent) {
    if (done || !isActivePointer(ev)) return
    opts.onMove((opts.axis === 'x' ? ev.clientX : ev.clientY) - start)
  }

  function end() {
    if (done) return
    done = true
    if (activeDrags.get(ownerDocument) === drag) {
      activeDrags.delete(ownerDocument)
    }
    ownerDocument.body.style.cursor = prevCursor
    ownerDocument.body.style.userSelect = prevSelect
    ownerWindow.removeEventListener('pointermove', move)
    ownerWindow.removeEventListener('pointerup', endPointer)
    ownerWindow.removeEventListener('pointercancel', endPointer)
    ownerWindow.removeEventListener('blur', end)
    target?.removeEventListener('lostpointercapture', endPointer)
    try {
      if (target?.hasPointerCapture(e.pointerId)) {
        target.releasePointerCapture(e.pointerId)
      }
    } catch {
      /* already released */
    }
    opts.onEnd?.()
  }

  function endPointer(ev: PointerEvent) {
    if (isActivePointer(ev)) end()
  }

  ownerWindow.addEventListener('pointermove', move)
  ownerWindow.addEventListener('pointerup', endPointer)
  ownerWindow.addEventListener('pointercancel', endPointer)
  ownerWindow.addEventListener('blur', end)
  target?.addEventListener('lostpointercapture', endPointer)

  const drag = { cancel: end }
  activeDrags.set(ownerDocument, drag)

  return drag
}
