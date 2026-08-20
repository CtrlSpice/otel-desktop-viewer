// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { render, screen } from '@testing-library/svelte'
import ExpandableValueHarness from '@/test/ExpandableValueHarness.svelte'

/**
 * jsdom reports every layout measurement as 0, so "is this clipped" cannot be
 * observed here -- that part is verified in a real browser. What these cover is
 * everything around the measurement: that the control appears only when
 * something is hidden, that toggling changes the clamp, and that expansion does
 * not survive a change of value.
 */
function setClipped(clipped: boolean) {
  // The component compares scrollHeight to clientHeight; jsdom leaves both at
  // 0, so the element is never clipped unless we say so.
  Object.defineProperty(HTMLElement.prototype, 'scrollHeight', {
    configurable: true,
    get() {
      return clipped ? 100 : 0
    },
  })
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
    configurable: true,
    get() {
      return 0
    },
  })
}

describe('ExpandableValue', () => {
  it('offers no control when nothing is hidden', async () => {
    setClipped(false)
    render(ExpandableValueHarness, { value: 'short' })
    await tick()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('offers the control only when something is hidden', async () => {
    setClipped(true)
    render(ExpandableValueHarness, { value: 'a'.repeat(500) })
    await tick()
    expect(
      screen.getByRole('button', { name: 'Show more' })
    ).toBeInTheDocument()
  })

  it('toggles the clamp and the label', async () => {
    setClipped(true)
    const { container } = render(ExpandableValueHarness, {
      value: 'a'.repeat(500),
    })
    await tick()
    const clamped = () =>
      !!container.querySelector('.detail-pair__value--clamped')

    expect(clamped()).toBe(true)
    screen.getByRole('button', { name: 'Show more' }).click()
    await tick()
    expect(clamped()).toBe(false)
    screen.getByRole('button', { name: 'Show less' }).click()
    await tick()
    expect(clamped()).toBe(true)
  })

  // Clicking between spans should not leave a field open from the previous one:
  // the same field on the next span is a different question.
  it('collapses again when the value changes', async () => {
    setClipped(true)
    const { container, rerender } = render(ExpandableValueHarness, {
      value: 'a'.repeat(500),
    })
    await tick()
    screen.getByRole('button', { name: 'Show more' }).click()
    await tick()
    expect(container.querySelector('.detail-pair__value--clamped')).toBeNull()

    await rerender({ value: 'b'.repeat(500) })
    await tick()
    expect(
      container.querySelector('.detail-pair__value--clamped')
    ).not.toBeNull()
  })
})
