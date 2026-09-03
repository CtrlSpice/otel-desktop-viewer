// @vitest-environment jsdom
import { describe, expect, it, vi, afterEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/svelte'
import RecentTimeRanges from './RecentTimeRanges.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { type RecentTimeRange } from '@/utils/time'

const RECENT_STORAGE_KEY = 'datetime-filter-recent'

function renderComponent() {
  setTestUrl('/traces')
  return renderWithContexts(RecentTimeRanges)
}

describe('RecentTimeRanges', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows an empty state when no recent ranges exist', () => {
    renderComponent()
    expect(screen.getByText('No recent time ranges')).toBeInTheDocument()
  })

  it('renders seeded recent ranges and applies one when clicked', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))

    const recents: RecentTimeRange[] = [
      {
        start: Date.now() - 600_000,
        end: Date.now(),
        usedAt: Date.now(),
      },
      {
        start: Date.now() - 1_200_000,
        end: Date.now() - 600_000,
        usedAt: Date.now() - 1000,
      },
    ]
    localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(recents))

    renderComponent()

    const list = screen.getByRole('list', {
      name: 'Recently used time ranges',
    })
    expect(list).toHaveClass('recent-range-list')
    const buttons = screen.getAllByRole('button')
    expect(buttons).toHaveLength(2)
    expect(buttons[0]).toHaveAttribute('aria-pressed', 'false')
    expect(screen.queryByText('No recent time ranges')).not.toBeInTheDocument()

    await fireEvent.click(buttons[0])

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved).toMatchObject({
      type: 'recent',
      start: recents[0].start,
      end: recents[0].end,
    })
    expect(buttons[0]).toHaveAttribute('aria-pressed', 'true')
    expect(window.location.search).toContain(`start=${recents[0].start}`)
    expect(window.location.search).toContain(`end=${recents[0].end}`)
  })
})
