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

function down(el: HTMLElement, x = 100, y = 100): PointerEvent {
  const e = new MouseEvent('pointerdown', {
    clientX: x,
    clientY: y,
    bubbles: true,
    cancelable: true,
  }) as unknown as PointerEvent
  Object.defineProperty(e, 'pointerId', { value: 1 })
  Object.defineProperty(e, 'currentTarget', { value: el })
  return e
}

function moveTo(x: number, y = 100) {
  window.dispatchEvent(
    new MouseEvent('pointermove', { clientX: x, clientY: y })
  )
}

afterEach(() => {
  document.body.innerHTML = ''
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
})

describe('startDrag', () => {
  it('reports a signed pixel delta along its axis', () => {
    const onMove = vi.fn()
    startDrag(down(handle(), 100), { axis: 'x', onMove })
    moveTo(160)
    moveTo(40)
    expect(onMove).toHaveBeenNthCalledWith(1, 60)
    expect(onMove).toHaveBeenNthCalledWith(2, -60)
  })

  it('measures the other axis when asked', () => {
    const onMove = vi.fn()
    startDrag(down(handle(), 100, 100), { axis: 'y', onMove })
    moveTo(999, 130)
    expect(onMove).toHaveBeenCalledWith(30)
  })

  it('holds the cursor and suppresses selection for the whole drag', () => {
    // The point of the exercise: mid-drag the pointer is over other elements,
    // whose cursor and text selection would otherwise win.
    const h = handle()
    startDrag(down(h), { axis: 'x', onMove: () => {} })
    expect(document.body.style.cursor).toBe('col-resize')
    expect(document.body.style.userSelect).toBe('none')
    window.dispatchEvent(new MouseEvent('pointerup'))
    expect(document.body.style.cursor).toBe('')
    expect(document.body.style.userSelect).toBe('')
  })

  it('restores whatever the body had before, not a guess', () => {
    document.body.style.cursor = 'wait'
    startDrag(down(handle()), { axis: 'x', onMove: () => {} })
    window.dispatchEvent(new MouseEvent('pointerup'))
    expect(document.body.style.cursor).toBe('wait')
  })

  it('captures the pointer so a fast drag does not escape the handle', () => {
    const h = handle()
    startDrag(down(h), { axis: 'x', onMove: () => {} })
    expect(h.setPointerCapture).toHaveBeenCalledWith(1)
    window.dispatchEvent(new MouseEvent('pointerup'))
    expect(h.releasePointerCapture).toHaveBeenCalledWith(1)
  })

  it('survives a browser refusing capture', () => {
    const h = handle()
    h.setPointerCapture = vi.fn(() => {
      throw new Error('nope')
    })
    const onMove = vi.fn()
    expect(() => startDrag(down(h), { axis: 'x', onMove })).not.toThrow()
    moveTo(150)
    expect(onMove).toHaveBeenCalledWith(50)
  })

  it('ends once, whichever way the drag finishes', () => {
    const onEnd = vi.fn()
    startDrag(down(handle()), { axis: 'x', onMove: () => {}, onEnd })
    window.dispatchEvent(new MouseEvent('pointerup'))
    window.dispatchEvent(new MouseEvent('pointercancel'))
    expect(onEnd).toHaveBeenCalledTimes(1)
  })

  it('stops moving after it ends', () => {
    const onMove = vi.fn()
    startDrag(down(handle()), { axis: 'x', onMove })
    window.dispatchEvent(new MouseEvent('pointerup'))
    moveTo(500)
    expect(onMove).not.toHaveBeenCalled()
  })

  it('cancel() releases the body for a component unmounting mid-drag', () => {
    const onEnd = vi.fn()
    const d = startDrag(down(handle()), { axis: 'x', onMove: () => {}, onEnd })
    d.cancel()
    expect(document.body.style.userSelect).toBe('')
    expect(onEnd).toHaveBeenCalledTimes(1)
  })
})
