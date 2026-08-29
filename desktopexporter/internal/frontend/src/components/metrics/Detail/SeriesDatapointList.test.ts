// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { tick } from 'svelte'
import type { SumDataPoint } from '@/types/api-types'
import type { MetricViewContext } from '@/contexts/metric-view-context.svelte'
import SeriesDatapointListHarness from '@/test/SeriesDatapointListHarness.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { SPAN_PARAM } from '@/route/query-params'

const navigateToItem = vi.hoisted(() => vi.fn())

vi.mock('@/route', async importOriginal => {
  const actual = await importOriginal<typeof import('@/route')>()
  return { ...actual, navigateToItem }
})

function makeDatapoint(overrides: Partial<SumDataPoint> = {}): SumDataPoint {
  return {
    id: 'dp-1',
    timestamp: 1_700_000_000_000_000_000n,
    timestampMs: 1_700_000_000_000,
    startTime: 1_700_000_000_000_000_000n,
    flags: 0,
    metricType: 'Sum',
    doubleValue: 42,
    intValue: null,
    valueType: 'double',
    isMonotonic: true,
    aggregationTemporality: 'Cumulative',
    exemplars: [
      {
        value: 42,
        timestamp: 1_700_000_000_000_000_000n,
        traceID: 'trace-ex',
        spanID: 'span-ex',
        filteredAttributes: [],
      },
    ],
    ...overrides,
  }
}

function makeDatapoints(count: number): SumDataPoint[] {
  return Array.from({ length: count }, (_, index) => {
    const timestamp = 1_700_000_000_000_000_000n + BigInt(index) * 1_000_000n
    return makeDatapoint({
      id: `dp-${index + 1}`,
      timestamp,
      timestampMs: Number(timestamp / 1_000_000n),
      doubleValue: index + 1,
      exemplars: [],
    })
  })
}

function renderedDatapointIDs(): string[] {
  return [
    ...document.querySelectorAll<HTMLTableRowElement>('tr[data-dp-id]'),
  ].map(row => row.dataset.dpId!)
}

function inspectButton(id: string): HTMLButtonElement {
  const button = document.querySelector<HTMLButtonElement>(
    `tr[data-dp-id="${id}"] button`
  )
  expect(button, `Inspect button for ${id} should exist`).not.toBeNull()
  return button!
}

function renderListView(datapoints: SumDataPoint[]) {
  let context: MetricViewContext | undefined
  const oncontext = (ctx: MetricViewContext) => {
    context = ctx
  }
  const view = renderWithContexts(SeriesDatapointListHarness, {
    datapoints,
    oncontext,
  })
  if (!context) throw new Error('harness did not report a metric view context')
  return {
    context,
    rerender: (nextDatapoints: SumDataPoint[]) =>
      view.rerender({
        componentProps: { datapoints: nextDatapoints, oncontext },
      }),
  }
}

function renderList(datapoints: SumDataPoint[]): MetricViewContext {
  return renderListView(datapoints).context
}

describe('SeriesDatapointList pagination and keyboard access', () => {
  beforeEach(() => {
    navigateToItem.mockClear()
    setTestUrl('/metrics/m1?start=0&end=1')
  })

  it('shows 25 rows by default in incoming order with a visible range', () => {
    const datapoints = makeDatapoints(61).reverse()
    renderList(datapoints)

    expect(
      screen.getByRole('table', { name: 'Datapoints' })
    ).toBeInTheDocument()
    expect(
      screen
        .getAllByRole('columnheader')
        .map(header => header.textContent?.trim())
    ).toEqual(['Time', 'Value', 'Details', 'Action'])
    expect(renderedDatapointIDs()).toEqual(
      datapoints.slice(0, 25).map(dp => dp.id)
    )
    expect(screen.getByText('1-25 of 61 datapoints')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Rows per page' })).toHaveValue(
      '25'
    )
    expect(
      screen
        .getAllByRole('option')
        .map(option => (option as HTMLOptionElement).value)
    ).toEqual(['25', '50', '100'])
    expect(
      screen.getAllByRole('button', { name: /^Inspect datapoint at / })
    ).toHaveLength(25)
    expect(inspectButton('dp-61')).toHaveAccessibleName(/value 61, ID dp-61$/)
    expect(inspectButton('dp-61')).toHaveTextContent('Inspect')
  })

  it('gives same-millisecond, same-value datapoints unique accessible names', () => {
    const timestamp = 1_700_000_000_000_000_000n
    renderList([
      makeDatapoint({
        id: 'dp-same-1',
        timestamp: timestamp + 1n,
        timestampMs: 1_700_000_000_000,
        exemplars: [],
      }),
      makeDatapoint({
        id: 'dp-same-2',
        timestamp: timestamp + 2n,
        timestampMs: 1_700_000_000_000,
        exemplars: [],
      }),
    ])

    const firstName = inspectButton('dp-same-1').getAttribute('aria-label')
    const secondName = inspectButton('dp-same-2').getAttribute('aria-label')
    expect(firstName).toMatch(/value 42, ID dp-same-1$/)
    expect(secondName).toMatch(/value 42, ID dp-same-2$/)
    expect(firstName).not.toBe(secondName)
  })

  it('uses DetailNav for first, previous, next, and last pages', async () => {
    const user = userEvent.setup()
    const datapoints = makeDatapoints(61)
    renderList(datapoints)

    expect(screen.getByRole('button', { name: 'First page' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Next page' }))
    expect(renderedDatapointIDs()).toEqual(
      datapoints.slice(25, 50).map(dp => dp.id)
    )
    expect(screen.getByText('26-50 of 61 datapoints')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Last page' }))
    expect(renderedDatapointIDs()).toEqual(
      datapoints.slice(50).map(dp => dp.id)
    )
    expect(screen.getByText('51-61 of 61 datapoints')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Last page' })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Previous page' }))
    expect(screen.getByText('26-50 of 61 datapoints')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'First page' }))
    expect(screen.getByText('1-25 of 61 datapoints')).toBeInTheDocument()
  })

  it('allows every paginator move while a datapoint remains selected', async () => {
    const user = userEvent.setup()
    const datapoints = makeDatapoints(61)
    const context = renderList(datapoints)
    await user.click(inspectButton('dp-1'))

    await user.click(screen.getByRole('button', { name: 'Next page' }))
    expect(screen.getByText('26-50 of 61 datapoints')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Last page' }))
    expect(screen.getByText('51-61 of 61 datapoints')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Previous page' }))
    expect(screen.getByText('26-50 of 61 datapoints')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'First page' }))
    expect(screen.getByText('1-25 of 61 datapoints')).toBeInTheDocument()
    expect(context.selectedDatapointID).toBe('dp-1')
  })

  it('offers 50 and 100 row page sizes', async () => {
    const user = userEvent.setup()
    const datapoints = makeDatapoints(120)
    renderList(datapoints)
    const pageSize = screen.getByRole('combobox', { name: 'Rows per page' })

    await user.selectOptions(pageSize, '50')
    expect(renderedDatapointIDs()).toEqual(
      datapoints.slice(0, 50).map(dp => dp.id)
    )
    expect(screen.getByText('1-50 of 120 datapoints')).toBeInTheDocument()

    await user.selectOptions(pageSize, '100')
    expect(renderedDatapointIDs()).toEqual(
      datapoints.slice(0, 100).map(dp => dp.id)
    )
    expect(screen.getByText('1-100 of 120 datapoints')).toBeInTheDocument()
  })

  it('keeps the page-size reset after selecting a datapoint on the last page', async () => {
    const user = userEvent.setup()
    const datapoints = makeDatapoints(61)
    const context = renderList(datapoints)

    await user.click(screen.getByRole('button', { name: 'Last page' }))
    await user.click(inspectButton('dp-61'))
    await user.selectOptions(
      screen.getByRole('combobox', { name: 'Rows per page' }),
      '50'
    )

    expect(screen.getByText('1-50 of 61 datapoints')).toBeInTheDocument()
    expect(renderedDatapointIDs()).toEqual(
      datapoints.slice(0, 50).map(datapoint => datapoint.id)
    )
    expect(context.selectedDatapointID).toBe('dp-61')
  })

  it('moves focus without inspecting and leaves Enter and Space to the button', async () => {
    const user = userEvent.setup()
    const context = renderList(makeDatapoints(3))
    const first = inspectButton('dp-1')
    const second = inspectButton('dp-2')

    first.focus()
    await user.keyboard('{ArrowDown}')
    expect(second).toHaveFocus()
    expect(context.selectedDatapointID).toBeNull()

    await user.keyboard('{Enter}')
    expect(context.selectedDatapointID).toBe('dp-2')
    expect(second).toHaveAttribute('aria-pressed', 'true')

    await user.keyboard(' ')
    expect(context.selectedDatapointID).toBeNull()
    expect(second).toHaveAttribute('aria-pressed', 'false')
  })

  it('moves focus across page boundaries without activating a row', async () => {
    const user = userEvent.setup()
    const context = renderList(makeDatapoints(26))

    inspectButton('dp-25').focus()
    await user.keyboard('{ArrowDown}')

    await waitFor(() => expect(inspectButton('dp-26')).toHaveFocus())
    expect(renderedDatapointIDs()).toEqual(['dp-26'])
    expect(context.selectedDatapointID).toBeNull()

    await user.keyboard('{ArrowUp}')
    await waitFor(() => expect(inspectButton('dp-25')).toHaveFocus())
    expect(renderedDatapointIDs()[0]).toBe('dp-1')
    expect(context.selectedDatapointID).toBeNull()
  })

  it('moves keyboard focus across page boundaries without following the active selection', async () => {
    const user = userEvent.setup()
    const context = renderList(makeDatapoints(26))

    await user.click(inspectButton('dp-25'))
    await user.keyboard('{ArrowDown}')

    await waitFor(() => expect(inspectButton('dp-26')).toHaveFocus())
    expect(renderedDatapointIDs()).toEqual(['dp-26'])
    expect(context.selectedDatapointID).toBe('dp-25')

    await user.keyboard('{ArrowUp}')
    await waitFor(() => expect(inspectButton('dp-25')).toHaveFocus())
    expect(screen.getByText('1-25 of 26 datapoints')).toBeInTheDocument()
    expect(context.selectedDatapointID).toBe('dp-25')
  })

  it('moves focus to an externally selected Inspect button when the old page is removed', async () => {
    const datapoints = makeDatapoints(60)
    const context = renderList(datapoints)
    inspectButton('dp-1').focus()

    context.onDatapointClick(datapoints[39]!)

    await waitFor(() => expect(inspectButton('dp-40')).toHaveFocus())
    expect(screen.getByText('26-50 of 60 datapoints')).toBeInTheDocument()
  })

  it('moves focus to the nearest remaining Inspect button when the page clamps', async () => {
    const user = userEvent.setup()
    const datapoints = makeDatapoints(51)
    const view = renderListView(datapoints)
    await user.click(screen.getByRole('button', { name: 'Last page' }))
    inspectButton('dp-51').focus()

    await view.rerender(datapoints.slice(0, 30))

    await waitFor(() => expect(inspectButton('dp-30')).toHaveFocus())
    expect(screen.getByText('26-30 of 30 datapoints')).toBeInTheDocument()
  })

  it('reveals a selected datapoint without stealing focus', async () => {
    const datapoints = makeDatapoints(60)
    const context = renderList(datapoints)
    const outside = document.createElement('button')
    document.body.append(outside)
    outside.focus()

    context.onDatapointClick(datapoints[39]!)

    await waitFor(() =>
      expect(screen.getByText('26-50 of 60 datapoints')).toBeInTheDocument()
    )
    expect(inspectButton('dp-40')).toHaveAttribute('aria-pressed', 'true')
    expect(outside).toHaveFocus()
    outside.remove()
  })

  it('re-reveals a selected datapoint when replacement data moves it', async () => {
    const datapoints = makeDatapoints(60)
    const view = renderListView(datapoints)
    view.context.onDatapointClick(datapoints[0]!)
    await tick()

    await view.rerender([...datapoints.slice(1), datapoints[0]!])

    await waitFor(() =>
      expect(screen.getByText('51-60 of 60 datapoints')).toBeInTheDocument()
    )
    expect(inspectButton('dp-1')).toHaveAttribute('aria-pressed', 'true')
  })

  it('does not rescan every datapoint when selection changes', async () => {
    const datapoints = makeDatapoints(5_000)
    let idReads = 0
    for (const datapoint of datapoints) {
      const id = datapoint.id
      Object.defineProperty(datapoint, 'id', {
        configurable: true,
        get() {
          idReads++
          return id
        },
      })
    }
    const context = renderList(datapoints)
    await tick()
    idReads = 0

    context.onDatapointClick(datapoints[4_999]!)

    await waitFor(() =>
      expect(inspectButton('dp-5000')).toHaveAttribute('aria-pressed', 'true')
    )
    // Rendering the destination page reads its visible rows. A linear lookup
    // would add another 5,000 reads before that render.
    expect(idReads).toBeLessThan(1_000)
  })

  it('expands details for a flags-only datapoint through Inspect', async () => {
    const user = userEvent.setup()
    renderList([makeDatapoint({ id: 'dp-flags', flags: 4, exemplars: [] })])
    const inspect = inspectButton('dp-flags')

    expect(inspect).toHaveAttribute('aria-expanded', 'false')
    await user.click(inspect)

    expect(inspect).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('4')).toBeInTheDocument()
  })

  it('expands and collapses exemplar details through Inspect', async () => {
    const user = userEvent.setup()
    renderList([makeDatapoint()])
    const inspect = inspectButton('dp-1')

    await user.click(inspect)
    expect(inspect).toHaveAttribute('aria-expanded', 'true')
    expect(
      screen.getByRole('link', { name: 'trace: trace-ex' })
    ).toBeInTheDocument()

    await user.click(inspect)
    expect(inspect).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.queryByRole('link', { name: 'trace: trace-ex' })
    ).not.toBeInTheDocument()
  })
})

describe('SeriesDatapointList exemplar trace correlation', () => {
  beforeEach(() => {
    navigateToItem.mockClear()
    setTestUrl('/metrics/m1?start=0&end=1')
  })

  it('links exemplar trace and span ids with span in the href', () => {
    renderWithContexts(SeriesDatapointListHarness, {
      datapoints: [makeDatapoint()],
      expandDatapointID: 'dp-1',
    })
    expect(
      screen.getByRole('link', { name: 'trace: trace-ex' })
    ).toHaveAttribute('href', '/traces/trace-ex?start=0&end=1&span=span-ex')
    expect(screen.getByRole('link', { name: 'span: span-ex' })).toHaveAttribute(
      'href',
      '/traces/trace-ex?start=0&end=1&span=span-ex'
    )
  })

  // The store caps how many exemplars a datapoint ships. The reader has to be
  // able to tell a datapoint that held three from one that held three hundred,
  // or the missing trace links look like traces that were never recorded.
  it('says how many exemplars were withheld when the store capped the list', () => {
    renderWithContexts(SeriesDatapointListHarness, {
      datapoints: [makeDatapoint({ exemplarCount: 64 })],
      expandDatapointID: 'dp-1',
    })
    expect(screen.getByText(/1 of 64 ex/)).toBeInTheDocument()
    expect(screen.getByText(/showing 1 of 64/)).toBeInTheDocument()
  })

  it('names only the count it has when nothing was withheld', () => {
    // No exemplarCount at all, which is what the store sends when the list is
    // complete -- the overwhelmingly common case, and the one that must not
    // render a "showing 1 of undefined" notice.
    renderWithContexts(SeriesDatapointListHarness, {
      datapoints: [makeDatapoint()],
      expandDatapointID: 'dp-1',
    })
    expect(screen.getByText('1 ex')).toBeInTheDocument()
    expect(screen.queryByText(/showing/)).not.toBeInTheDocument()
  })

  it('navigates with span patch when an exemplar span link is clicked', async () => {
    renderWithContexts(SeriesDatapointListHarness, {
      datapoints: [makeDatapoint()],
      expandDatapointID: 'dp-1',
    })
    await userEvent.click(screen.getByRole('link', { name: 'span: span-ex' }))
    expect(navigateToItem).toHaveBeenCalledWith('traces', 'trace-ex', 'push', {
      [SPAN_PARAM]: 'span-ex',
    })
  })
})
