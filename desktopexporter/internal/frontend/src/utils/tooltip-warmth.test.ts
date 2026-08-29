// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  createTooltipWarmth,
  initTooltipWarmth,
  TOOLTIP_WARM_GRACE_MS,
} from './tooltip-warmth'

describe('createTooltipWarmth', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('is not instant on the first enter', () => {
    const setInstant = vi.fn()
    const { enter } = createTooltipWarmth(setInstant)
    enter()
    expect(setInstant).toHaveBeenCalledWith(false)
  })

  it('is instant on a second enter while still warm', () => {
    const setInstant = vi.fn()
    const { enter, leave } = createTooltipWarmth(setInstant)
    enter()
    leave()
    enter()
    expect(setInstant).toHaveBeenLastCalledWith(true)
  })

  it('cools back down after the grace period with nothing hovered', () => {
    const setInstant = vi.fn()
    const { enter, leave } = createTooltipWarmth(setInstant)
    enter()
    leave()
    vi.advanceTimersByTime(TOOLTIP_WARM_GRACE_MS)
    setInstant.mockClear()

    enter()
    expect(setInstant).toHaveBeenCalledWith(false)
  })

  it('stays warm if re-entered before the grace period elapses', () => {
    const setInstant = vi.fn()
    const { enter, leave } = createTooltipWarmth(setInstant)
    enter()
    leave()
    vi.advanceTimersByTime(TOOLTIP_WARM_GRACE_MS - 1)
    setInstant.mockClear()

    enter()
    expect(setInstant).toHaveBeenCalledWith(true)
  })

  it('does not start cooling while a second trigger is still active', () => {
    const setInstant = vi.fn()
    const { enter, leave } = createTooltipWarmth(setInstant)
    enter() // A
    enter() // B, while A still active
    leave() // A leaves, B still active
    vi.advanceTimersByTime(TOOLTIP_WARM_GRACE_MS)
    setInstant.mockClear()

    enter() // re-enter A: should still be warm, since B never left
    expect(setInstant).toHaveBeenCalledWith(true)
  })

  it('never lets the active count go negative on an unmatched leave', () => {
    const setInstant = vi.fn()
    const { enter, leave } = createTooltipWarmth(setInstant)
    leave()
    leave()
    enter()
    expect(setInstant).toHaveBeenCalledWith(false)
  })

  it('destroy clears the pending cool-down timer', () => {
    const setInstant = vi.fn()
    const { enter, leave, destroy } = createTooltipWarmth(setInstant)
    enter()
    leave()
    destroy()
    vi.advanceTimersByTime(TOOLTIP_WARM_GRACE_MS)
    setInstant.mockClear()

    enter()
    // destroy only stops the pending timer, it doesn't force a cool-down
    expect(setInstant).toHaveBeenCalledWith(true)
  })

  it('reset immediately cools the next tooltip without losing active counts', () => {
    const setInstant = vi.fn()
    const { enter, leave, reset } = createTooltipWarmth(setInstant)
    enter()
    reset()
    expect(setInstant).toHaveBeenLastCalledWith(false)

    leave()
    enter()
    expect(setInstant).toHaveBeenLastCalledWith(false)
  })
})

describe('initTooltipWarmth', () => {
  let root: HTMLElement
  let target: HTMLElement
  let cleanup: () => void

  beforeEach(() => {
    vi.useFakeTimers()
    root = document.createElement('div')
    root.innerHTML = `
      <button class="tooltip" data-tip="Home" id="a"></button>
      <button class="tooltip" data-tip="Settings" id="b"></button>
      <button id="plain"></button>
    `
    document.body.appendChild(root)
    target = document.createElement('div')
    cleanup = initTooltipWarmth(root, target)
  })

  afterEach(() => {
    cleanup()
    root.remove()
    vi.useRealTimers()
  })

  function pointerOver(el: Element) {
    el.dispatchEvent(new Event('pointerover', { bubbles: true }))
  }
  function pointerOut(el: Element) {
    el.dispatchEvent(new Event('pointerout', { bubbles: true }))
  }

  function focusIn(el: Element) {
    el.dispatchEvent(new FocusEvent('focusin', { bubbles: true }))
  }
  function focusOut(el: Element) {
    el.dispatchEvent(new FocusEvent('focusout', { bubbles: true }))
  }

  it('sets the instant delay only on a second, still-warm trigger', () => {
    const a = root.querySelector('#a')!
    const b = root.querySelector('#b')!

    pointerOver(a)
    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('')

    pointerOut(a)
    pointerOver(b)
    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('0s')
  })

  it('ignores pointer events outside any tooltip trigger', () => {
    pointerOver(root.querySelector('#plain')!)
    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('')
  })

  it('restores the delay after the grace period with nothing hovered', () => {
    const a = root.querySelector('#a')!
    pointerOver(a)
    pointerOut(a)
    vi.advanceTimersByTime(TOOLTIP_WARM_GRACE_MS)
    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('')
  })

  it('suppresses a focused tooltip on Escape and resets warmth', () => {
    const a = root.querySelector('#a')!
    const b = root.querySelector('#b')!
    pointerOver(a)
    pointerOut(a)
    focusIn(b)
    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('0s')

    b.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
    )
    expect(b).toHaveAttribute('data-tooltip-suppressed')
    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('')

    focusOut(b)
    focusIn(a)
    expect(a).not.toHaveAttribute('data-tooltip-suppressed')
    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('')
  })

  it('keeps an escaped tooltip suppressed until pointer and focus both leave', () => {
    const a = root.querySelector('#a')!
    pointerOver(a)
    focusIn(a)
    a.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
    )

    focusOut(a)
    expect(a).toHaveAttribute('data-tooltip-suppressed')
    pointerOut(a)
    expect(a).not.toHaveAttribute('data-tooltip-suppressed')
  })

  it('drops active state for a tooltip removed before its leave events', () => {
    const a = root.querySelector('#a')!
    const b = root.querySelector('#b')!
    pointerOver(a)
    focusIn(a)
    a.remove()

    pointerOver(b)
    focusIn(b)
    pointerOut(b)
    focusOut(b)
    vi.advanceTimersByTime(TOOLTIP_WARM_GRACE_MS)

    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('')
  })

  it('cleanup removes listeners and any instant override', () => {
    const a = root.querySelector('#a')!
    const b = root.querySelector('#b')!
    pointerOver(a)
    pointerOut(a)
    pointerOver(b)
    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('0s')

    cleanup()
    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('')
    expect(b).not.toHaveAttribute('data-tooltip-suppressed')

    pointerOut(b)
    vi.advanceTimersByTime(TOOLTIP_WARM_GRACE_MS)
    pointerOver(a)
    expect(target.style.getPropertyValue('--tooltip-show-delay')).toBe('')
  })
})
