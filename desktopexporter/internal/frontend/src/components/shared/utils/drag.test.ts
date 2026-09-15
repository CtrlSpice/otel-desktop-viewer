// @vitest-environment jsdom
import { describe, expect, it, vi, afterEach } from 'vitest'
import { startDrag } from './drag'

function handle() {
  const el = document.createElement('div')
  el.setPointerCapture = vi.fn()
  el.releasePointerCapture = vi.fn()
  el.hasPointerCapture = vi.fn(() => true)
  document.body.appendChild(el)
  return el
}

function pointerEvent(type: string, pointerId: number, x = 100, y = 100) {
  const e = Object.assign(
    new MouseEvent(type, {
      clientX: x,
      clientY: y,
      bubbles: true,
      cancelable: true,
    }),
    { pointerId }
  )
  return e
}

function down(el: HTMLElement, pointerId = 1, x = 100, y = 100) {
  const e = pointerEvent('pointerdown', pointerId, x, y)
  Object.defineProperty(e, 'currentTarget', { value: el })
  return e
}

function dispatchPointer(type: string, pointerId: number, x = 100, y = 100) {
  window.dispatchEvent(pointerEvent(type, pointerId, x, y))
}

afterEach(() => {
  document.body.innerHTML = ''
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
})

describe('startDrag', () => {
  it('reports a signed pixel delta along its axis', () => {
    const onMove = vi.fn()
    startDrag(down(handle(), 1, 100), { axis: 'x', onMove })
    dispatchPointer('pointermove', 1, 160)
    dispatchPointer('pointermove', 1, 40)
    dispatchPointer('pointerup', 1)
    expect(onMove).toHaveBeenNthCalledWith(1, 60)
    expect(onMove).toHaveBeenNthCalledWith(2, -60)
  })

  it('measures the other axis when asked', () => {
    const onMove = vi.fn()
    startDrag(down(handle(), 1, 100, 100), { axis: 'y', onMove })
    dispatchPointer('pointermove', 1, 999, 130)
    dispatchPointer('pointerup', 1)
    expect(onMove).toHaveBeenCalledWith(30)
  })

  it('holds the cursor and suppresses selection for the whole drag', () => {
    // The point of the exercise: mid-drag the pointer is over other elements,
    // whose cursor and text selection would otherwise win.
    const h = handle()
    startDrag(down(h), { axis: 'x', onMove: () => {} })
    expect(document.body.style.cursor).toBe('col-resize')
    expect(document.body.style.userSelect).toBe('none')
    dispatchPointer('pointerup', 1)
    expect(document.body.style.cursor).toBe('')
    expect(document.body.style.userSelect).toBe('')
  })

  it('restores whatever the body had before, not a guess', () => {
    document.body.style.cursor = 'wait'
    startDrag(down(handle()), { axis: 'x', onMove: () => {} })
    dispatchPointer('pointerup', 1)
    expect(document.body.style.cursor).toBe('wait')
  })

  it('captures the pointer so a fast drag does not escape the handle', () => {
    const h = handle()
    startDrag(down(h), { axis: 'x', onMove: () => {} })
    expect(h.setPointerCapture).toHaveBeenCalledWith(1)
    dispatchPointer('pointerup', 1)
    expect(h.releasePointerCapture).toHaveBeenCalledWith(1)
  })

  it('survives a browser refusing capture', () => {
    const h = handle()
    h.setPointerCapture = vi.fn(() => {
      throw new Error('nope')
    })
    const onMove = vi.fn()
    expect(() => startDrag(down(h), { axis: 'x', onMove })).not.toThrow()
    dispatchPointer('pointermove', 1, 150)
    expect(onMove).toHaveBeenCalledWith(50)
    dispatchPointer('pointerup', 1)
  })

  it('ends once, whichever way the drag finishes', () => {
    const onEnd = vi.fn()
    startDrag(down(handle()), { axis: 'x', onMove: () => {}, onEnd })
    dispatchPointer('pointerup', 1)
    dispatchPointer('pointercancel', 1)
    expect(onEnd).toHaveBeenCalledTimes(1)
  })

  it('stops moving after it ends', () => {
    const onMove = vi.fn()
    startDrag(down(handle()), { axis: 'x', onMove })
    dispatchPointer('pointerup', 1)
    dispatchPointer('pointermove', 1, 500)
    expect(onMove).not.toHaveBeenCalled()
  })

  it('cancel() releases the body for a component unmounting mid-drag', () => {
    const onEnd = vi.fn()
    const d = startDrag(down(handle()), { axis: 'x', onMove: () => {}, onEnd })
    d.cancel()
    expect(document.body.style.userSelect).toBe('')
    expect(onEnd).toHaveBeenCalledTimes(1)
  })

  it('ignores move, end, and cancellation events from other pointers', () => {
    const onMove = vi.fn()
    const onEnd = vi.fn()
    startDrag(down(handle(), 7), { axis: 'x', onMove, onEnd })

    dispatchPointer('pointermove', 8, 160)
    dispatchPointer('pointerup', 8)
    dispatchPointer('pointercancel', 8)
    expect(onMove).not.toHaveBeenCalled()
    expect(onEnd).not.toHaveBeenCalled()

    dispatchPointer('pointermove', 7, 160)
    dispatchPointer('pointerup', 7)
    expect(onMove).toHaveBeenCalledWith(60)
    expect(onEnd).toHaveBeenCalledTimes(1)
  })

  it('ends on blur and removes its capture and listeners', () => {
    const h = handle()
    const onMove = vi.fn()
    const onEnd = vi.fn()
    startDrag(down(h, 7), { axis: 'x', onMove, onEnd })

    window.dispatchEvent(new Event('blur'))
    dispatchPointer('pointermove', 7, 160)
    dispatchPointer('pointerup', 7)
    expect(onMove).not.toHaveBeenCalled()
    expect(onEnd).toHaveBeenCalledTimes(1)
    expect(h.releasePointerCapture).toHaveBeenCalledWith(7)
    expect(document.body.style.cursor).toBe('')
    expect(document.body.style.userSelect).toBe('')
  })

  it('only ends for the captured pointer losing capture', () => {
    const h = handle()
    const onEnd = vi.fn()
    startDrag(down(h, 7), { axis: 'x', onMove: () => {}, onEnd })

    h.dispatchEvent(pointerEvent('lostpointercapture', 8))
    expect(onEnd).not.toHaveBeenCalled()
    h.dispatchEvent(pointerEvent('lostpointercapture', 7))
    expect(onEnd).toHaveBeenCalledTimes(1)
    expect(h.releasePointerCapture).toHaveBeenCalledWith(7)
  })

  it('cancels the former owner before a new drag takes body styles', () => {
    document.body.style.cursor = 'wait'
    document.body.style.userSelect = 'text'
    const firstEnd = vi.fn()
    const secondEnd = vi.fn()
    startDrag(down(handle(), 1), {
      axis: 'x',
      onMove: () => {},
      onEnd: firstEnd,
    })
    startDrag(down(handle(), 2), {
      axis: 'x',
      onMove: () => {},
      onEnd: secondEnd,
    })

    expect(firstEnd).toHaveBeenCalledTimes(1)
    expect(document.body.style.cursor).toBe('col-resize')
    expect(document.body.style.userSelect).toBe('none')
    dispatchPointer('pointerup', 2)
    expect(secondEnd).toHaveBeenCalledTimes(1)
    expect(document.body.style.cursor).toBe('wait')
    expect(document.body.style.userSelect).toBe('text')
  })

  it('reserves ownership while cancelling the former owner', () => {
    const reentrantEnd = vi.fn()
    const requestedEnd = vi.fn()
    function restartOnEnd() {
      reentrantEnd()
      startDrag(down(handle(), 3), {
        axis: 'x',
        onMove: () => {},
        onEnd: restartOnEnd,
      })
    }
    startDrag(down(handle(), 1), {
      axis: 'x',
      onMove: () => {},
      onEnd: restartOnEnd,
    })
    startDrag(down(handle(), 2), {
      axis: 'x',
      onMove: () => {},
      onEnd: requestedEnd,
    })

    expect(reentrantEnd).toHaveBeenCalledTimes(1)
    dispatchPointer('pointerup', 3)
    expect(reentrantEnd).toHaveBeenCalledTimes(1)
    dispatchPointer('pointerup', 2)
    expect(requestedEnd).toHaveBeenCalledTimes(1)
    expect(document.body.style.cursor).toBe('')
    expect(document.body.style.userSelect).toBe('')
  })
})
