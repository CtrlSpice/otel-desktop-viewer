// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import HomePage, { rejectionLabel } from './HomePage.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

describe('rejectionLabel', () => {
  it('labels known rejection kinds', () => {
    expect(rejectionLabel('span_already_stored')).toBe('duplicate span id')
    expect(rejectionLabel('span_refused')).toBe('rejected by the store')
  })

  it('falls back to unknown and prototype-named kinds', () => {
    expect(rejectionLabel('new_rejection')).toBe('new_rejection')
    expect(rejectionLabel('toString')).toBe('toString')
    expect(rejectionLabel('constructor')).toBe('constructor')
  })
})

describe('signal overview disclosures', () => {
  afterEach(() => vi.restoreAllMocks())

  it('keeps navigation links visible and independently focusable when collapsed', async () => {
    vi.spyOn(telemetryAPI, 'getStats').mockResolvedValue({
      traces: {
        traceCount: 0,
        spanCount: 0,
        serviceCount: 0,
        errorCount: 0,
        lastReceived: null,
      },
      metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
      logs: { logCount: 0, errorCount: 0, lastReceived: null },
      rejections: [],
    })
    setTestUrl('/')
    const user = userEvent.setup()
    renderWithContexts(HomePage)

    for (const label of ['Traces', 'Metrics', 'Logs']) {
      const link = screen.getByRole('link', {
        name: `View all ${label.toLowerCase()}`,
      })
      const details = screen
        .getByRole('table', { name: `${label} overview` })
        .closest('details')!
      const summary = details.querySelector('summary')!

      expect(link).toHaveAttribute('href', `/${label.toLowerCase()}`)
      expect(link.closest('details')).toBeNull()
      expect(summary.querySelector('a, button, input, [tabindex]')).toBeNull()
      expect(details.open).toBe(true)

      await fireEvent.click(summary)
      expect(details.open).toBe(false)
      expect(link).toBeVisible()

      link.focus()
      expect(link).toHaveFocus()
      await user.tab()
      expect(summary).toHaveFocus()

      await fireEvent.click(summary)
      expect(details.open).toBe(true)
    }
  })
})
