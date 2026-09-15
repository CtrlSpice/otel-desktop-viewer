// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/svelte'
import ThemeToggle from '../components/shared/ThemeToggle.svelte'

describe('Svelte transition fallback', () => {
  const animate = Element.prototype.animate

  afterEach(() => {
    Element.prototype.animate = animate
  })

  it('completes an animation on the next microtask', async () => {
    const animation = document.createElement('div').animate([], { duration: 0 })
    let finished = false
    animation.onfinish = () => {
      finished = true
    }

    await Promise.resolve()

    expect(finished).toBe(true)
  })

  it('does not complete an animation cancelled before that microtask', async () => {
    const animation = document.createElement('div').animate([], { duration: 0 })
    let finished = false
    animation.onfinish = () => {
      finished = true
    }
    animation.cancel()

    await Promise.resolve()

    expect(finished).toBe(false)
  })

  it('keeps main durations finite when reversing a keyed fade', async () => {
    const calls: Array<{ options: KeyframeAnimationOptions }> = []
    Element.prototype.animate = function (_keyframes, options) {
      calls.push({ options: options as KeyframeAnimationOptions })
      return animate.call(this, _keyframes, options)
    }

    render(ThemeToggle)
    const button = screen.getByRole('button')

    await fireEvent.click(button)
    await Promise.resolve()
    await fireEvent.click(button)
    await Promise.resolve()
    await Promise.resolve()

    const mainCalls = calls.filter(call => call.options.duration !== 0)
    expect(mainCalls).not.toHaveLength(0)
    expect(
      mainCalls.every(call => Number.isFinite(call.options.duration))
    ).toBe(true)
  })
})
