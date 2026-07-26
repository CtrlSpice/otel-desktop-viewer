// @vitest-environment jsdom
import { describe, expect, it, vi, afterEach } from 'vitest'
import { screen, fireEvent, waitFor } from '@testing-library/svelte'
import { tick } from 'svelte'
import DateTimeFilter from './DateTimeFilter.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

// The popover JS API comes from the shared polyfill in src/test/setup.ts.

function getPopover() {
  return document.querySelector('[popover="auto"]') as HTMLElement
}

function renderComponent() {
  setTestUrl('/traces')
  return renderWithContexts(DateTimeFilter)
}

afterEach(() => {
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
