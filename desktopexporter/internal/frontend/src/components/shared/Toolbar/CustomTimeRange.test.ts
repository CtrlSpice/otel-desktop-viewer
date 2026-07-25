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
})
