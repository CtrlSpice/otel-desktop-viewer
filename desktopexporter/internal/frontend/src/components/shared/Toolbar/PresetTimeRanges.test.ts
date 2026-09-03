// @vitest-environment jsdom
import { describe, expect, it, afterEach, vi } from 'vitest'
import { screen, fireEvent } from '@testing-library/svelte'
import PresetTimeRanges from './PresetTimeRanges.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

const PRESET_LABELS = ['All', '5m', '15m', '30m', '1h', '6h', '24h', '7d']

function renderComponent() {
  setTestUrl('/traces')
  return renderWithContexts(PresetTimeRanges)
}

function getPresetButton(label: string) {
  return screen.getByRole('button', {
    name: label === 'All' ? 'All time' : `Last ${label}`,
  })
}

describe('PresetTimeRanges', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders every preset button', () => {
    renderComponent()
    for (const label of PRESET_LABELS) {
      expect(getPresetButton(label)).toBeInTheDocument()
    }
  })

  it('selects the All preset by default', () => {
    renderComponent()
    expect(getPresetButton('All')).toHaveAttribute('aria-pressed', 'true')
    for (const label of PRESET_LABELS.slice(1)) {
      expect(getPresetButton(label)).toHaveAttribute('aria-pressed', 'false')
    }
  })

  it('applies a duration preset and updates the time context', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    fireEvent.click(getPresetButton('5m'))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved).toMatchObject({
      type: 'preset',
      presetIndex: 1,
      durationMs: 300_000,
    })
    expect(window.location.search).toContain(`start=${Date.now() - 300_000}`)
    expect(window.location.search).toContain(`end=${Date.now()}`)
    expect(getPresetButton('5m')).toHaveAttribute('aria-pressed', 'true')
    expect(getPresetButton('All')).toHaveAttribute('aria-pressed', 'false')
  })

  it('applies All as an explicit unbounded selection', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    fireEvent.click(getPresetButton('All'))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved).toEqual({ type: 'all' })
    expect(window.location.search).toBe('?time=all')
    expect(localStorage.getItem('datetime-filter-recent')).toBeNull()
  })
})
