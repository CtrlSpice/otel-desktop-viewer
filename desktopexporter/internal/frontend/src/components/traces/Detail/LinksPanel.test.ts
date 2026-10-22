// @vitest-environment jsdom
import { describe, expect, it, beforeEach, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import LinksPanel from './LinksPanel.svelte'
import type { LinkData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

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
    setTestUrl('/traces/trace-1?start=0&end=1&span=current-span&event=3')
  })

  it('shows only the destination name and service while keeping IDs in expanded details', async () => {
    renderWithContexts(LinksPanel, {
      links: [
        makeLink({
          summary: {
            target: { name: 'consume order', serviceName: 'worker' },
          },
        }),
      ],
    })
    const card = screen.getByRole('button', { expanded: false })
    expect(card).toHaveTextContent('consume order (worker)')
    expect(card).not.toHaveTextContent('linked-trace')
    expect(card).not.toHaveTextContent('linked-span')
    expect(card).not.toHaveTextContent('→')
    expect(card).toHaveTextContent('3 fields')
    await userEvent.click(card)
    expect(screen.getByRole('link', { name: 'linked-span' })).toHaveAttribute(
      'href',
      '/traces/linked-trace?start=0&end=1&span=linked-span'
    )
  })

  it('uses both target IDs when the target is not stored', () => {
    renderWithContexts(LinksPanel, {
      links: [
        makeLink({
          summary: {
            target: null,
          },
        }),
      ],
    })
    expect(screen.getByRole('button')).toHaveAccessibleName(
      'Traces: Trace ID linked-trace Span ID linked-span (unavailable) · Link 3 fields'
    )
  })

  it('does not invent service names or hide empty names', () => {
    renderWithContexts(LinksPanel, {
      links: [
        makeLink({
          summary: {
            target: { name: '', serviceName: '' },
          },
        }),
      ],
    })
    expect(screen.getByRole('button')).toHaveTextContent('linked-span')
    expect(screen.getByRole('button')).not.toHaveTextContent('()')
  })

  it('keeps invalid linked identities explicit when the destination is unresolved', () => {
    renderWithContexts(LinksPanel, {
      links: [
        makeLink({
          traceID: null,
          summary: {
            target: null,
          },
        }),
      ],
    })
    expect(screen.getByRole('button')).toHaveTextContent('Invalid link target')
    expect(screen.queryByText('(unavailable)')).toBeNull()
    expect(screen.queryByRole('link')).toBeNull()
  })

  it('links trace and span ids with span in the href', async () => {
    renderWithContexts(LinksPanel, { links: [makeLink()] })
    const card = screen.getByRole('button', { expanded: false })
    expect(card).toHaveTextContent('linked-trace')
    expect(card).toHaveTextContent('linked-span')
    expect(screen.queryByText('(unavailable)')).toBeNull()
    expect(screen.queryByRole('link')).toBeNull()
    await userEvent.click(card)
    expect(screen.getByRole('link', { name: 'linked-trace' })).toHaveAttribute(
      'href',
      '/traces/linked-trace?start=0&end=1&span=linked-span'
    )
    expect(screen.getByRole('link', { name: 'linked-span' })).toHaveAttribute(
      'href',
      '/traces/linked-trace?start=0&end=1&span=linked-span'
    )
  })

  it('drops stale trace state when navigating to a linked span', async () => {
    renderWithContexts(LinksPanel, { links: [makeLink()] })
    const historyLength = window.history.length

    await userEvent.click(screen.getByRole('button', { expanded: false }))
    await userEvent.click(screen.getByRole('link', { name: 'linked-span' }))

    expect(window.location.pathname).toBe('/traces/linked-trace')
    expect(window.location.search).toBe('?start=0&end=1&span=linked-span')
    expect(window.history.length).toBe(historyLength + 1)
  })

  it('reserves copying linked fields for the copy button while link clicks navigate', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const { container } = renderWithContexts(LinksPanel, {
      links: [makeLink()],
    })
    await user.click(screen.getByRole('button', { expanded: false }))
    const originalURL = window.location.href
    const field = container.querySelector('[data-field="spanId"]')!
    await user.click(field.querySelector('.detail-property__header')!)
    await user.click(field.querySelector('.detail-property__body')!)
    expect(copy).not.toHaveBeenCalled()
    expect(window.location.href).toBe(originalURL)

    await user.click(screen.getByRole('button', { name: 'Copy spanId' }))
    expect(copy).toHaveBeenCalledExactlyOnceWith('linked-span')
    expect(window.location.href).toBe(originalURL)
    copy.mockClear()

    await user.click(screen.getByRole('link', { name: 'linked-span' }))
    expect(window.location.pathname).toBe('/traces/linked-trace')
    expect(window.location.search).toBe('?start=0&end=1&span=linked-span')
    expect(copy).not.toHaveBeenCalled()
  })

  it('keeps an invalid link without inventing zero IDs or navigation', () => {
    renderWithContexts(LinksPanel, {
      links: [makeLink({ traceID: null, spanID: null })],
    })

    expect(screen.getByText('Invalid link target')).toBeInTheDocument()
    const warning = document.querySelector('.links-panel__warning')
    expect(warning).toHaveAttribute('title', 'Invalid link target')
    expect(warning).toHaveAttribute('aria-hidden', 'true')
    expect(screen.getAllByText('null')).toHaveLength(2)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('links only the trace when the linked span ID is missing', async () => {
    renderWithContexts(LinksPanel, {
      links: [makeLink({ spanID: null })],
    })

    await userEvent.click(screen.getByRole('button', { expanded: false }))
    expect(screen.getByRole('link', { name: 'linked-trace' })).toHaveAttribute(
      'href',
      '/traces/linked-trace?start=0&end=1'
    )
    expect(screen.getByText('null').closest('a')).toBeNull()
  })

  it('opens link cards independently without navigating from their summaries', async () => {
    renderWithContexts(LinksPanel, {
      links: [makeLink(), makeLink({ traceID: 'another-trace' })],
    })
    const cards = screen.getAllByRole('button', { expanded: false })
    await userEvent.click(cards[0]!)
    cards[1]!.focus()
    await userEvent.keyboard('{Enter}')
    expect(cards[0]).toHaveAttribute('aria-expanded', 'true')
    expect(cards[1]).toHaveAttribute('aria-expanded', 'true')
    expect(window.location.pathname).toBe('/traces/trace-1')
    await userEvent.click(cards[0]!)
    expect(cards[1]).toHaveAttribute('aria-expanded', 'true')
  })

  it('updates an unavailable target in place while preserving disclosure and exact copy values', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const link = makeLink({
      traceID: 'A'.repeat(32),
      spanID: 'B'.repeat(16),
      traceState: 'vendor=recorded',
      summary: { target: null },
    })
    const view = renderWithContexts(LinksPanel, { links: [link] })
    const card = screen.getByRole('button', { expanded: false })
    expect(card).toHaveTextContent('(unavailable)')
    await user.click(card)
    await user.click(screen.getByRole('button', { name: 'Copy spanId' }))
    expect(copy).toHaveBeenCalledWith(link.spanID)
    expect(screen.getByRole('link', { name: link.spanID! })).toHaveAttribute(
      'href',
      `/traces/${link.traceID}?start=0&end=1&span=${link.spanID}`
    )
    await view.rerender({
      componentProps: {
        links: [
          {
            ...link,
            summary: {
              target: { name: 'Arrived target', serviceName: 'worker' },
            },
          },
        ],
      },
    })
    expect(screen.getByRole('button', { expanded: true })).toBe(card)
    expect(card).toHaveTextContent('Arrived target (worker)')
    expect(screen.queryByText('(unavailable)')).toBeNull()
    expect(screen.getByText('vendor=recorded')).toBeVisible()
    expect(screen.getByRole('link', { name: link.spanID! })).toBeVisible()
    copy.mockRestore()
  })
})

describe('LinksPanel field count', () => {
  beforeEach(() => setTestUrl('/traces/trace-1?start=0&end=1'))

  const rows = () => document.querySelectorAll('tr[data-field]').length
  const badge = () =>
    document
      .querySelector('.signal-row__badge .badge-count:last-child')
      ?.textContent?.trim()

  it('counts every rendered row when the link carries flags', () => {
    renderWithContexts(LinksPanel, {
      links: [makeLink({ flags: 1, attributes: [] })],
    })
    expect(document.body.textContent).toContain('flags')
    expect(badge()).toBe(`${rows()} fields`)
  })

  it('still agrees when there are no flags', () => {
    renderWithContexts(LinksPanel, {
      links: [makeLink({ flags: 0, attributes: [] })],
    })
    expect(document.body.textContent).not.toContain('flags')
    expect(badge()).toBe(`${rows()} fields`)
  })

  it('counts duplicate typed attributes and dropped-attribute metadata without merging rows', () => {
    renderWithContexts(LinksPanel, {
      links: [
        makeLink({
          flags: 1,
          droppedAttributesCount: 2,
          attributes: [
            { key: 'attempt', value: { kind: 'int64', value: 1n } },
            { key: 'attempt', value: { kind: 'string', value: '1' } },
          ],
        }),
      ],
    })
    expect(rows()).toBe(7)
    expect(badge()).toBe('7 fields')
    expect(document.querySelectorAll('[data-field="attempt"]')).toHaveLength(2)
  })
})
