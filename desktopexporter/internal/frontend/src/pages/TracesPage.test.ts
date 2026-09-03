// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import TracesPage from './TracesPage.svelte'
import type {
  TraceSummary,
  TraceData,
  Stats,
  SpanNode,
} from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

// TracesPage is the only place the unplaced-span warning banner is rendered
// -- the singular/plural copy and the `unplacedSpanCount > 0` gate both live
// inline in its template, not in an extracted helper. Mounting the whole
// page (with telemetryAPI stubbed) is the cheapest seam that actually
// exercises that markup: pulling the copy logic out into a testable
// function would be a production change the task asked us not to make, and
// nothing smaller than the page owns this conditional.
//
// The heavy pieces underneath (DrawerSearchPanel's CodeMirror editor, the
// virtual list) already mount successfully in jsdom elsewhere in this suite
// without special stand-ins (see DrawerSearchPanel.test.ts and
// SignalListDrawer.test.ts), so nothing extra is mocked here beyond the
// three telemetryAPI calls TracesPage itself drives.

const { searchTraces, getStats, searchSpans, getTraceAttributes } = vi.hoisted(
  () => ({
    searchTraces: vi.fn(),
    getStats: vi.fn(),
    searchSpans: vi.fn(),
    getTraceAttributes: vi.fn(),
  })
)

vi.mock('@/services/telemetry-service', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/services/telemetry-service')>()
  return {
    ...actual,
    telemetryAPI: {
      ...actual.telemetryAPI,
      searchTraces,
      getStats,
      searchSpans,
      getTraceAttributes,
    },
  }
})

function makeTraceSummary(overrides: Partial<TraceSummary> = {}): TraceSummary {
  return {
    traceID: 'trace-1',
    hasRootSpan: true,
    rootSpan: { name: 'GET /orders', serviceName: 'orders-service' },
    startTime: 1_700_000_000_000_000_000n,
    durationNs: 1_000_000n,
    spanCount: 1,
    errorCount: 0,
    ...overrides,
  }
}

function makeStats(): Stats {
  return {
    traces: {
      traceCount: 1,
      spanCount: 1,
      serviceCount: 1,
      errorCount: 0,
      lastReceived: null,
    },
    logs: { logCount: 0, errorCount: 0, lastReceived: null },
    metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
    rejections: [],
  }
}

function makeSpanNode(id: string): SpanNode {
  return {
    depth: 0,
    matched: true,
    spanData: {
      spanID: id,
      flags: 0,
      parentSpanID: null,
      traceID: 'trace-1',
      name: id,
      startTime: 0n,
      endTime: 1_000_000n,
      statusCode: 'Ok',
      events: [],
      links: [],
      attributes: [],
      resource: { attributes: [], droppedAttributesCount: 0 },
      scope: {
        name: '',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
      },
      traceState: '',
      kind: 'Server',
      droppedAttributesCount: 0,
      droppedEventsCount: 0,
      droppedLinksCount: 0,
      statusMessage: '',
    },
  }
}

function makeTraceData(unplacedSpanCount: number): TraceData {
  return {
    traceID: 'trace-1',
    unplacedSpanCount,
    spans: [makeSpanNode('root')],
  }
}

async function renderSelectedTrace(unplacedSpanCount: number) {
  searchTraces.mockResolvedValue([makeTraceSummary()])
  getStats.mockResolvedValue(makeStats())
  searchSpans.mockResolvedValue(makeTraceData(unplacedSpanCount))
  setTestUrl('/traces/trace-1')
  renderWithContexts(TracesPage)
  // Wait for the detail fetch to land -- the waterfall's root row is proof
  // traceData is populated and the banner conditional has had its chance to
  // run, without coupling the wait to the banner itself.
  await waitFor(() =>
    expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
  )
}

beforeEach(() => {
  if (typeof Element.prototype.scrollTo !== 'function') {
    Element.prototype.scrollTo = () => {}
  }
  searchTraces.mockReset()
  getStats.mockReset()
  searchSpans.mockReset()
  getTraceAttributes.mockReset()
  getTraceAttributes.mockResolvedValue([])
})

/** Collapses the template's line-wrapped whitespace into single spaces so
 *  assertions read (and match) as one sentence. */
function normalizedText(el: HTMLElement): string {
  return el.textContent!.replace(/\s+/g, ' ').trim()
}

describe('TracesPage unplaced spans banner', () => {
  it('queries the list with null bounds for the default All selection', async () => {
    await renderSelectedTrace(0)
    expect(searchTraces).toHaveBeenCalledWith(null, null)
  })

  it('renders no banner when every span was placed', async () => {
    await renderSelectedTrace(0)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('renders the banner with singular copy for exactly one unplaced span', async () => {
    await renderSelectedTrace(1)
    const alert = await screen.findByRole('alert')
    expect(normalizedText(alert)).toContain('1 span is missing from this trace')
    expect(normalizedText(alert)).not.toContain('spans are missing')
  })

  it('renders the banner with plural copy for more than one unplaced span', async () => {
    await renderSelectedTrace(3)
    const alert = await screen.findByRole('alert')
    expect(normalizedText(alert)).toContain(
      '3 spans are missing from this trace'
    )
    expect(normalizedText(alert)).not.toContain('3 span is')
  })
})
