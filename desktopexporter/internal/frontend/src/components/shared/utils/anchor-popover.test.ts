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
})
