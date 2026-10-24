// @vitest-environment jsdom
import { afterEach, expect, it } from 'vitest'
import { screen } from '@testing-library/svelte'
import { renderWithContexts } from '@/test/render-helpers'
import ChartTimeRangeHeader from './ChartTimeRangeHeader.svelte'

afterEach(() => localStorage.removeItem('time-tz'))

it('shows shared date and zone once while retaining both complete timestamps', () => {
  localStorage.setItem('time-tz', 'UTC')
  renderWithContexts(ChartTimeRangeHeader, {
    startMs: Date.parse('2026-10-19T09:53:39.470Z'),
    endMs: Date.parse('2026-10-19T09:54:03.470Z'),
  })
  expect(screen.getAllByText('2026-10-19')).toHaveLength(1)
  expect(screen.getAllByText('UTC')).toHaveLength(1)
  expect(
    screen.getByLabelText('Start: 2026-10-19 09:53:39.470 UTC')
  ).toHaveAttribute('datetime', '2026-10-19T09:53:39.470Z')
  expect(
    screen.getByLabelText('End: 2026-10-19 09:54:03.470 UTC')
  ).toHaveTextContent('09:54:03.470')
})

it('shows both dates when the range crosses local midnight', () => {
  localStorage.setItem('time-tz', 'America/Los_Angeles')
  renderWithContexts(ChartTimeRangeHeader, {
    startMs: Date.parse('2026-10-20T06:59:59.000Z'),
    endMs: Date.parse('2026-10-20T07:00:01.000Z'),
  })
  expect(
    screen.getByLabelText('Start: 2026-10-19 23:59:59.000 PDT')
  ).toHaveTextContent('2026-10-19')
  expect(
    screen.getByLabelText('End: 2026-10-20 00:00:01.000 PDT')
  ).toHaveTextContent('2026-10-20')
  expect(screen.getAllByText('PDT')).toHaveLength(1)
})

it('keeps each zone visible when repeated clock times cross the DST fallback', () => {
  localStorage.setItem('time-tz', 'America/Los_Angeles')
  renderWithContexts(ChartTimeRangeHeader, {
    startMs: Date.parse('2026-11-01T08:30:00.000Z'),
    endMs: Date.parse('2026-11-01T09:30:00.000Z'),
  })
  expect(screen.getAllByText('2026-11-01')).toHaveLength(1)
  expect(
    screen.getByLabelText('Start: 2026-11-01 01:30:00.000 PDT')
  ).toHaveTextContent('PDT')
  expect(
    screen.getByLabelText('End: 2026-11-01 01:30:00.000 PST')
  ).toHaveTextContent('PST')
})
