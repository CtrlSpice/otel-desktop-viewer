// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import LinksPanel from './LinksPanel.svelte'
import type { LinkData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { SPAN_PARAM } from '@/route/query-params'

const navigateToItem = vi.hoisted(() => vi.fn())

vi.mock('@/route', async importOriginal => {
  const actual = await importOriginal<typeof import('@/route')>()
  return { ...actual, navigateToItem }
})

function makeLink(overrides: Partial<LinkData> = {}): LinkData {
  return {
    traceID: 'linked-trace',
    spanID: 'linked-span',
    traceState: '',
    attributes: [],
    droppedAttributesCount: 0,
    flags: 0,
    ...overrides,
  }
}

describe('LinksPanel trace correlation', () => {
  beforeEach(() => {
    navigateToItem.mockClear()
    setTestUrl('/traces/trace-1?start=0&end=1')
  })

  it('links trace and span ids with span in the href', () => {
    renderWithContexts(LinksPanel, { links: [makeLink()] })
    expect(screen.getByRole('link', { name: 'linked-trace' })).toHaveAttribute(
      'href',
      '/traces/linked-trace?start=0&end=1&span=linked-span'
    )
    expect(screen.getByRole('link', { name: 'linked-span' })).toHaveAttribute(
      'href',
      '/traces/linked-trace?start=0&end=1&span=linked-span'
    )
  })

  it('navigates with span patch when a span link is clicked', async () => {
    renderWithContexts(LinksPanel, { links: [makeLink()] })
    await userEvent.click(screen.getByRole('link', { name: 'linked-span' }))
    expect(navigateToItem).toHaveBeenCalledWith(
      'traces',
      'linked-trace',
      'push',
      { [SPAN_PARAM]: 'linked-span' }
    )
  })
})

// The header badge counts the rows the panel renders, and the two are computed
// in different places -- linkFieldCount() by arithmetic, the rows by markup --
// so nothing but a test holds them together. Adding the flags row without
// adding it to the count went unnoticed precisely because every fixture here
// used flags: 0, where the arithmetic happens to be right either way.
describe('LinksPanel field count', () => {
  beforeEach(() => setTestUrl('/traces/trace-1?start=0&end=1'))

  const rows = () => document.querySelectorAll('tr.table-row').length
  const badge = () =>
    document.querySelector('.badge-count')?.textContent?.trim()

  it('counts every rendered row when the link carries flags', () => {
    renderWithContexts(LinksPanel, {
      links: [makeLink({ flags: 1, attributes: [] })],
    })
    expect(document.body.textContent).toContain('flags')
    expect(badge()).toBe(String(rows()))
  })

  it('still agrees when there are no flags', () => {
    renderWithContexts(LinksPanel, {
      links: [makeLink({ flags: 0, attributes: [] })],
    })
    expect(document.body.textContent).not.toContain('flags')
    expect(badge()).toBe(String(rows()))
  })
})
