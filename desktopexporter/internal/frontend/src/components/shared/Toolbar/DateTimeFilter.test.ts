// @vitest-environment jsdom
import { describe, expect, it, vi, afterEach } from 'vitest'
import { screen, fireEvent, waitFor } from '@testing-library/svelte'
import { tick } from 'svelte'
import DateTimeFilter from './DateTimeFilter.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

// The popover JS API comes from the shared polyfill in src/test/setup.ts.

function getPopover(): HTMLElement {
  const popover = document.querySelector('[popover="auto"]')
  if (!(popover instanceof HTMLElement)) {
    throw new Error('Expected the time-range popover')
  }
  return popover
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
    expect(
      screen.queryByText('Time range', { exact: true })
    ).not.toBeInTheDocument()
  })

  it('renders presets, custom range, and timezone without recent ranges', async () => {
    renderComponent()
    await tick()
    const popover = getPopover()
    popover.showPopover()

    expect(
      screen.getByRole('toolbar', { name: 'Time range presets' })
    ).toBeInTheDocument()
    expect(popover).not.toHaveTextContent('Custom Range')
    expect(screen.getByLabelText('Start')).toBeVisible()
    expect(screen.getByLabelText('End')).toBeVisible()
    expect(popover).toHaveTextContent('Timezone')
    expect(popover).not.toHaveTextContent('Recent')
  })

  it('updates the compact tooltip without changing the exact selected range', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-16T04:59:50Z'))
    localStorage.setItem('time-tz', 'America/New_York')
    const start = Date.parse('2026-01-15T15:00:00.123Z')
    const end = Date.parse('2026-01-15T16:30:00.456Z')
    setTestUrl(`/traces?start=${start}&end=${end}`)
    renderWithContexts(DateTimeFilter)
    await tick()

    const trigger = screen.getByRole('button', { name: /Change time range/i })
    const exactLabel =
      'Change time range, 2026-01-15, 10:00:00.123 - 2026-01-15, 11:30:00.456 EST'
    expect(trigger).toHaveTextContent(/^$/)
    expect(trigger).toHaveAccessibleName(exactLabel)
    expect(trigger.getAttribute('data-tip')?.replace(/\s+/g, ' ')).toBe(
      'Change time range, Today, 10:00 – 11:30'
    )

    await vi.advanceTimersByTimeAsync(30_000)
    await tick()
    expect(trigger.getAttribute('data-tip')?.replace(/\s+/g, ' ')).toBe(
      'Change time range, Jan 15, 10:00 – 11:30'
    )
    expect(trigger).toHaveAccessibleName(exactLabel)
    expect(window.location.search).toBe(`?start=${start}&end=${end}`)
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

    fireEvent.click(screen.getByRole('button', { name: 'Last 5 min' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved).toEqual({
      type: 'preset',
      durationMs: 300_000,
    })
    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
    )
    expect(trigger).toHaveAttribute('data-tip', 'Change time range, Last 5 min')
    expect(trigger).toHaveAccessibleName('Change time range, Last 5 min')
  })

  it('closes when All is clicked while All is already selected', async () => {
    renderComponent()
    await tick()
    const trigger = screen.getByRole('button', { name: /All time/i })
    const popover = getPopover()
    popover.showPopover()
    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'true')
    )

    fireEvent.click(screen.getByRole('button', { name: 'All time' }))

    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
    )
  })
})
