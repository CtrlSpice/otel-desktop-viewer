// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { positionAnchorPopover } from './anchor-popover'

function rect(values: Partial<DOMRect>): DOMRect {
  return {
    x: 0,
    y: 0,
    width: 0,
    height: 0,
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
    toJSON: () => ({}),
    ...values,
  }
}

describe('positionAnchorPopover', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('clamps a below-end popover inside the right viewport edge', () => {
    vi.stubGlobal('innerWidth', 390)
    const trigger = document.createElement('button')
    const popover = document.createElement('div')
    vi.spyOn(trigger, 'getBoundingClientRect').mockReturnValue(
      rect({ right: 399, bottom: 95 })
    )
    vi.spyOn(popover, 'getBoundingClientRect').mockReturnValue(
      rect({ left: 33, right: 399, width: 366 })
    )

    positionAnchorPopover(trigger, popover, 'below-end')

    expect(popover.style.left).toBe('16px')
    expect(popover.style.right).toBe('auto')
  })

  it('flips a component-capped below-end popover without enlarging it', () => {
    vi.stubGlobal('innerHeight', 500)
    const trigger = document.createElement('button')
    const popover = document.createElement('div')
    popover.style.maxHeight = '484px'
    vi.spyOn(trigger, 'getBoundingClientRect').mockReturnValue(
      rect({ top: 400, right: 200, bottom: 420 })
    )
    vi.spyOn(popover, 'getBoundingClientRect').mockImplementation(() => {
      expect(popover.style.maxHeight).toBe('')
      return rect({ height: 180, bottom: 608 })
    })

    positionAnchorPopover(trigger, popover, 'below-end')

    expect(popover.style.maxHeight).toBe('')
    expect(popover.style.top).toBe('212px')
  })

  it('caps and remeasures a below-end popover taller than the viewport', () => {
    vi.stubGlobal('innerHeight', 500)
    const trigger = document.createElement('button')
    const popover = document.createElement('div')
    vi.spyOn(trigger, 'getBoundingClientRect').mockReturnValue(
      rect({ top: 450, right: 200, bottom: 470 })
    )
    const getPopoverRect = vi
      .spyOn(popover, 'getBoundingClientRect')
      .mockReturnValueOnce(rect({ height: 600, bottom: 1078 }))
      .mockReturnValueOnce(rect({ height: 300, bottom: 778 }))

    positionAnchorPopover(trigger, popover, 'below-end')

    expect(popover.style.maxHeight).toBe('484px')
    expect(popover.style.top).toBe('142px')
    expect(getPopoverRect).toHaveBeenCalledTimes(2)
  })
})
