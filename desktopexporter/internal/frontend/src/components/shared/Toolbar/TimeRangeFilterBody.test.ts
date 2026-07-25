// @vitest-environment jsdom
import { describe, expect, it, vi, afterEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/svelte'
import TimeRangeFilterBody from './TimeRangeFilterBody.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function renderComponent() {
  setTestUrl('/traces')
  return renderWithContexts(TimeRangeFilterBody)
}

describe('TimeRangeFilterBody', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders the custom range, timezone, and recents sections', () => {
    renderComponent()
    expect(screen.getByText('Custom Range')).toBeInTheDocument()
    expect(screen.getByText('Timezone')).toBeInTheDocument()
    expect(screen.getByText('Recently Used')).toBeInTheDocument()
  })

  it('switches the timezone from local to UTC', () => {
    renderComponent()
    fireEvent.click(
      screen.getByRole('button', { name: /Coordinated Universal Time/i })
    )
    expect(localStorage.getItem('time-tz')).toBe('UTC')
  })

  it('applies a custom range through the embedded form', () => {
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
    expect(window.location.search).toContain(`start=${saved.start}`)
    expect(window.location.search).toContain(`end=${saved.end}`)
  })
})
