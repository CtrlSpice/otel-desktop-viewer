// @vitest-environment jsdom
import { describe, expect, it, vi, afterEach } from 'vitest'
import { tick } from 'svelte'
import { screen } from '@testing-library/svelte'
import type { TimeContext } from '@/contexts/time-context.svelte'
import { loadRecentTimeRanges } from '@/utils/time'
import TimeProbe from '@/test/TimeProbe.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

/** Renders the probe and returns the live time context it captured. */
function renderProbe(): TimeContext {
  let captured: TimeContext | undefined
  renderWithContexts(TimeProbe, {
    onContext: (context: TimeContext) => {
      captured = context
    },
  })
  if (!captured) throw new Error('TimeProbe did not report a context')
  return captured
}

function selectionType(): string {
  return screen.getByTestId('selection-type').textContent ?? ''
}

function selectionStart(): number {
  return Number(screen.getByTestId('selection-start').textContent)
}

function selectionEnd(): number {
  return Number(screen.getByTestId('selection-end').textContent)
}

function selectionPresetIndex(): string {
  return (screen.getByTestId('selection-preset-index').textContent ?? '').trim()
}

function reportedTz(): string {
  return screen.getByTestId('tz').textContent ?? ''
}

describe('time context default load', () => {
  it('loads the default preset (index 0, start 0) with no saved selection or URL params', () => {
    setTestUrl('/traces')
    renderProbe()
    expect(selectionType()).toBe('preset')
    expect(selectionPresetIndex()).toBe('0')
    expect(selectionStart()).toBe(0)
  })
})

describe('time context localStorage restore', () => {
  it('restores a saved custom selection from localStorage', () => {
    localStorage.setItem(
      'time-selection',
      JSON.stringify({ start: 111, end: 222, type: 'custom' })
    )
    setTestUrl('/traces')
    renderProbe()
    expect(selectionType()).toBe('custom')
    expect(selectionStart()).toBe(111)
    expect(selectionEnd()).toBe(222)
  })

  it('falls back to the default preset when the saved selection is corrupted JSON', () => {
    localStorage.setItem('time-selection', '{not valid json')
    setTestUrl('/traces')
    expect(() => renderProbe()).not.toThrow()
    expect(selectionType()).toBe('preset')
    expect(selectionPresetIndex()).toBe('0')
    expect(selectionStart()).toBe(0)
  })
})

describe('time context URL precedence', () => {
  it('prefers valid start/end URL params over a saved localStorage selection', () => {
    localStorage.setItem(
      'time-selection',
      JSON.stringify({ start: 111, end: 222, type: 'custom' })
    )
    setTestUrl('/traces?start=333&end=444')
    renderProbe()
    expect(selectionType()).toBe('custom')
    expect(selectionStart()).toBe(333)
    expect(selectionEnd()).toBe(444)
  })

  it('falls back to localStorage when the URL is missing the end param', () => {
    localStorage.setItem(
      'time-selection',
      JSON.stringify({ start: 111, end: 222, type: 'custom' })
    )
    setTestUrl('/traces?start=333')
    renderProbe()
    expect(selectionType()).toBe('custom')
    expect(selectionStart()).toBe(111)
    expect(selectionEnd()).toBe(222)
  })

  it('falls back to localStorage when the URL params are non-numeric', () => {
    localStorage.setItem(
      'time-selection',
      JSON.stringify({ start: 111, end: 222, type: 'custom' })
    )
    setTestUrl('/traces?start=abc&end=def')
    renderProbe()
    expect(selectionType()).toBe('custom')
    expect(selectionStart()).toBe(111)
    expect(selectionEnd()).toBe(222)
  })
})

describe('time context setSelection', () => {
  it('updates the reactive selection', async () => {
    setTestUrl('/traces')
    const context = renderProbe()
    context.setSelection(1000, 2000, 'custom')
    await tick()
    expect(selectionType()).toBe('custom')
    expect(selectionStart()).toBe(1000)
    expect(selectionEnd()).toBe(2000)
  })

  it('persists the selection to localStorage', () => {
    setTestUrl('/traces')
    const context = renderProbe()
    context.setSelection(1000, 2000, 'custom')
    expect(JSON.parse(localStorage.getItem('time-selection')!)).toEqual({
      start: 1000,
      end: 2000,
      type: 'custom',
    })
  })

  it('records a recent time range', () => {
    setTestUrl('/traces')
    const context = renderProbe()
    context.setSelection(1000, 2000, 'custom')
    const recents = loadRecentTimeRanges()
    expect(recents.some(r => r.start === 1000 && r.end === 2000)).toBe(true)
  })

  it('mirrors the window into the URL without adding a history entry', () => {
    setTestUrl('/traces')
    const context = renderProbe()
    const before = window.history.length
    context.setSelection(1000, 2000, 'custom')
    expect(window.history.length).toBe(before)
    const params = new URLSearchParams(window.location.search)
    expect(params.get('start')).toBe('1000')
    expect(params.get('end')).toBe('2000')
  })
})

describe('time context preset anchoring on write', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('mirrors a preset window ending at the current time with the preset duration', () => {
    const createdAt = new Date('2024-06-01T12:00:00Z').getTime()
    vi.useFakeTimers()
    vi.setSystemTime(createdAt)

    setTestUrl('/traces')
    const context = renderProbe()

    // Advance time between context creation and the write, to prove the
    // anchor is computed at write-time rather than at creation-time.
    const writtenAt = createdAt + 5 * 60_000
    vi.setSystemTime(writtenAt)

    const oneHourMs = 60 * 60_000
    context.setSelection(0, oneHourMs, 'preset', 2)

    const params = new URLSearchParams(window.location.search)
    expect(params.get('end')).toBe(String(writtenAt))
    expect(params.get('start')).toBe(String(writtenAt - oneHourMs))
  })
})

describe('time context external URL adoption', () => {
  it('adopts a URL window changed behind its back as a custom selection', async () => {
    setTestUrl('/traces?start=100&end=200')
    renderProbe()
    expect(selectionType()).toBe('custom')

    window.history.replaceState(null, '', '/traces?start=500&end=900')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await tick()

    expect(selectionType()).toBe('custom')
    expect(selectionStart()).toBe(500)
    expect(selectionEnd()).toBe(900)
  })
})

describe('time context own-write echo', () => {
  it('does not re-adopt the URL window it just wrote as an external change', async () => {
    setTestUrl('/traces')
    const context = renderProbe()

    context.setSelection(0, 60 * 60_000, 'preset', 1)
    await tick()
    expect(selectionType()).toBe('preset')

    // A popstate carrying the same window we just wrote (e.g. a stray
    // history event) must not flip the selection to 'custom'.
    window.dispatchEvent(new PopStateEvent('popstate'))
    await tick()

    expect(selectionType()).toBe('preset')
  })
})

describe('time context setTz', () => {
  it('updates the reactive tz and persists it to localStorage', async () => {
    setTestUrl('/traces')
    const context = renderProbe()
    context.setTz('UTC')
    await tick()
    expect(reportedTz()).toBe('UTC')
    expect(localStorage.getItem('time-tz')).toBe('UTC')
  })

  it('falls back to local when the saved tz value is invalid', () => {
    localStorage.setItem('time-tz', 'Mars/Standard')
    setTestUrl('/traces')
    renderProbe()
    expect(reportedTz()).toBe('local')
  })
})
