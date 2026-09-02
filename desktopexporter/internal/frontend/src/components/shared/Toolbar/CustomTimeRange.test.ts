// @vitest-environment jsdom
import { describe, expect, it, afterEach, vi } from 'vitest'
import { screen, fireEvent } from '@testing-library/svelte'
import CustomTimeRange from './CustomTimeRange.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function renderComponent() {
  setTestUrl('/traces')
  return renderWithContexts(CustomTimeRange)
}

describe('CustomTimeRange', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders empty start and "now" end inputs by default', () => {
    renderComponent()
    expect(screen.getByLabelText(/Start/i)).toHaveValue('')
    expect(screen.getByLabelText(/End/i)).toHaveValue('now')
  })

  it('applies a valid relative custom range', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    fireEvent.input(screen.getByLabelText(/Start/i), {
      target: { value: '2 hours ago' },
    })
    fireEvent.input(screen.getByLabelText(/End/i), {
      target: { value: '1 hour ago' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved.type).toBe('custom')
    expect(saved.start).toBeLessThan(saved.end)
    expect(saved.end).toBeLessThanOrEqual(Date.now())
    expect(window.location.search).toContain(`start=${saved.start}`)
    expect(window.location.search).toContain(`end=${saved.end}`)
  })

  it('shows an error when the start time is after the end time', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    fireEvent.input(screen.getByLabelText(/Start/i), {
      target: { value: '1 hour ago' },
    })
    fireEvent.input(screen.getByLabelText(/End/i), {
      target: { value: '2 hours ago' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    expect(
      screen.getByText('Start time must be before end time')
    ).toBeInTheDocument()
    expect(localStorage.getItem('time-selection')).toBeNull()
    expect(window.location.search).toBe('')
  })

  it('shows an error when the start time cannot be parsed', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    fireEvent.input(screen.getByLabelText(/Start/i), {
      target: { value: 'not a date' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    expect(
      screen.getByText('Could not understand this time format')
    ).toBeInTheDocument()
    expect(localStorage.getItem('time-selection')).toBeNull()
    expect(window.location.search).toBe('')
  })

  it('shows an error when the end time is in the future', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    fireEvent.input(screen.getByLabelText(/Start/i), {
      target: { value: '2 hours ago' },
    })
    fireEvent.input(screen.getByLabelText(/End/i), {
      target: { value: 'in 1 hour' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    expect(
      screen.getByText('End time cannot be in the future')
    ).toBeInTheDocument()
    expect(localStorage.getItem('time-selection')).toBeNull()
    expect(window.location.search).toBe('')
  })

  it('interprets absolute wall-clock input in a named timezone', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-16T00:00:00Z'))
    localStorage.setItem('time-tz', 'America/New_York')
    renderComponent()

    fireEvent.input(screen.getByLabelText(/Start/i), {
      target: { value: '2026-01-15, 07:00:00.000' },
    })
    fireEvent.input(screen.getByLabelText(/End/i), {
      target: { value: '2026-01-15, 08:00:00.000' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved.start).toBe(new Date('2026-01-15T12:00:00Z').getTime())
    expect(saved.end).toBe(new Date('2026-01-15T13:00:00Z').getTime())
  })

  it('round-trips an offset-disambiguated range during a DST overlap', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-02T00:00:00Z'))
    localStorage.setItem('time-tz', 'America/New_York')
    localStorage.setItem(
      'time-selection',
      JSON.stringify({
        type: 'custom',
        start: new Date('2026-11-01T05:30:00Z').getTime(),
        end: new Date('2026-11-01T07:30:00Z').getTime(),
      })
    )
    renderComponent()

    expect(screen.getByLabelText(/Start/i)).toHaveValue(
      '2026-11-01T01:30:00.000-04:00'
    )
    expect(screen.getByLabelText(/End/i)).toHaveValue(
      '2026-11-01T02:30:00.000-05:00'
    )
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved.start).toBe(new Date('2026-11-01T05:30:00Z').getTime())
    expect(saved.end).toBe(new Date('2026-11-01T07:30:00Z').getTime())
  })
})
