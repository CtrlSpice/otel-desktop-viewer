// @vitest-environment jsdom
import {
  describe,
  expect,
  it,
  vi,
  beforeAll,
  beforeEach,
  afterEach,
} from 'vitest'
import { screen, fireEvent, waitFor } from '@testing-library/svelte'
import { tick } from 'svelte'
import DateTimeFilter from './DateTimeFilter.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

class FakeToggleEvent extends Event {
  newState: 'open' | 'closed'
  constructor(type: string, init: { newState: 'open' | 'closed' }) {
    super(type, { bubbles: true })
    this.newState = init.newState
  }
}

const original = {
  showPopover: HTMLElement.prototype.showPopover,
  hidePopover: HTMLElement.prototype.hidePopover,
  togglePopover: HTMLElement.prototype.togglePopover,
}

function getPopover() {
  return document.querySelector('[popover="auto"]') as HTMLElement
}

function renderComponent() {
  setTestUrl('/traces')
  return renderWithContexts(DateTimeFilter)
}

beforeAll(() => {
  if (typeof window.requestAnimationFrame === 'undefined') {
    window.requestAnimationFrame = () => 0
    globalThis.requestAnimationFrame = window.requestAnimationFrame
  }
  if (typeof window.cancelAnimationFrame === 'undefined') {
    window.cancelAnimationFrame = () => undefined
    globalThis.cancelAnimationFrame = window.cancelAnimationFrame
  }
})

beforeEach(() => {
  HTMLElement.prototype.showPopover = function () {
    this.removeAttribute('popover')
    this.dispatchEvent(new FakeToggleEvent('toggle', { newState: 'open' }))
  }
  HTMLElement.prototype.hidePopover = function () {
    this.setAttribute('popover', 'auto')
    this.dispatchEvent(new FakeToggleEvent('toggle', { newState: 'closed' }))
  }
  HTMLElement.prototype.togglePopover = function () {
    const isOpen = !this.hasAttribute('popover')
    if (isOpen) {
      this.hidePopover()
    } else {
      this.showPopover()
    }
    return !isOpen
  }
})

afterEach(() => {
  HTMLElement.prototype.showPopover = original.showPopover
  HTMLElement.prototype.hidePopover = original.hidePopover
  HTMLElement.prototype.togglePopover = original.togglePopover
  vi.useRealTimers()
})

describe('DateTimeFilter', () => {
  it('renders a trigger button labelled with the current time range', () => {
    renderComponent()
    expect(
      screen.getByRole('button', { name: /Change time range/i })
    ).toBeInTheDocument()
  })

  it('renders the popover body with presets, custom range, timezone, and recents', async () => {
    renderComponent()
    await tick()
    const popover = getPopover()
    popover.showPopover()

    expect(
      screen.getByRole('toolbar', { name: 'Time range presets' })
    ).toBeInTheDocument()
    expect(popover).toHaveTextContent('Custom Range')
    expect(popover).toHaveTextContent('Timezone')
    expect(popover).toHaveTextContent('Recently Used')
  })

  it('opens the popover and applies a preset selection end-to-end', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()
    await tick()

    const trigger = screen.getByRole('button', { name: /Change time range/i })
    const popover = getPopover()

    popover.showPopover()
    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'true')
    )

    fireEvent.click(screen.getByRole('button', { name: 'Last 5m' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved).toMatchObject({
      type: 'preset',
      presetIndex: 1,
      start: Date.now() - 300_000,
      end: Date.now(),
    })
    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
    )
  })
})
