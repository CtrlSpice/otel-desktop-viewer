// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  waitFor,
  within,
} from '@testing-library/svelte'
import { afterEach, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import type {
  DiscoveryOptions,
  HomeCoveragePage,
  HomeCoverageValue,
} from '@/services/discovery-types'
import type { AttributeValue } from '@/types/api-types'
import { attributeValueCopy } from '@/components/shared/detail-value'
import AttributeDiscovery from './AttributeDiscovery.svelte'
import { copyText } from '@/utils/clipboard'

vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))

const service = {
  serviceName: 'checkout',
  serviceNamespace: '',
  traceCount: 100,
  spanCount: 100,
  errorSpanCount: 0,
  logCount: 0,
  errorLogCount: 0,
  logSeverityCounts: {
    trace: 0,
    debug: 0,
    info: 0,
    warn: 0,
    error: 0,
    fatal: 0,
    other: 0,
  },
  metricCount: 0,
  dataPointCount: 0,
  exemplarCount: 0,
  lastSeen: null,
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.mocked(copyText).mockReset()
})

it('does not restart a value request when another key owner finishes', async () => {
  type Page = Awaited<ReturnType<typeof telemetryAPI.discoverAttributeKeys>>
  const pending = new Map<
    string,
    ReturnType<typeof Promise.withResolvers<Page>>
  >()
  vi.spyOn(telemetryAPI, 'discoverAttributeKeys').mockImplementation(
    options => {
      const request = Promise.withResolvers<Page>()
      pending.set(options.ownerType!, request)
      return request.promise
    }
  )
  const coverage = Promise.withResolvers<HomeCoveragePage>()
  const values = vi
    .spyOn(telemetryAPI, 'discoverHomeAttributeValues')
    .mockReturnValue(coverage.promise)
  const props = {
    service,
    range: {},
    revision: 0,
    queries: { traces: null, logs: false as const, metrics: false as const },
    selectedAttribute: null,
    onSelectAttribute: vi.fn(),
  }
  const view = render(AttributeDiscovery, props)
  const foundOn = [{ signal: 'traces' as const, ownerType: 'span' as const }]
  pending.get('span')!.resolve({
    items: [{ key: 'ready', kind: 'string', foundOn }],
    truncated: false,
  })
  await waitFor(() => expect(values).toHaveBeenCalledTimes(1))
  const signal = values.mock.calls[0][1]!
  pending.get('event')!.resolve({ items: [], truncated: false })
  await tick()
  await tick()
  expect(values).toHaveBeenCalledTimes(1)
  expect(signal.aborted).toBe(false)
  pending.get('resource')!.resolve({
    items: [
      {
        key: 'sibling',
        kind: 'string',
        foundOn: [{ signal: 'traces', ownerType: 'resource' }],
      },
    ],
    truncated: false,
  })
  await view.findByRole('button', { name: 'Expand sibling · resource values' })
  expect(
    values.mock.calls.filter(([options]) => options.key === 'ready')
  ).toHaveLength(1)
  expect(signal.aborted).toBe(false)
  await view.rerender({ revision: 1 })
  expect(signal.aborted).toBe(true)
  expect(
    values.mock.calls.filter(([options]) => options.key === 'ready')
  ).toHaveLength(2)
  const current = values.mock.calls.at(-1)![1]!
  view.unmount()
  expect(current.aborted).toBe(true)
})

it.each(['success', 'failure', 'bulk'])(
  'retains expanded values across unrelated owner completion until their own replacement: %s',
  async outcome => {
    const foundOn = [{ signal: 'traces' as const, ownerType: 'span' as const }]
    const keys = vi
      .spyOn(telemetryAPI, 'discoverAttributeKeys')
      .mockImplementation(async options => ({
        items:
          options.ownerType === 'span'
            ? [{ key: 'retained', kind: 'string', foundOn }]
            : [],
        truncated: false,
      }))
    const original = homeResult([
      {
        identity: 'old',
        value: { kind: 'string', value: 'Original value' },
        foundOn,
        count: 1,
        kindCount: 1,
        denominator: 1,
        relativeFrequency: 1,
      },
    ])
    const values = vi
      .spyOn(telemetryAPI, 'discoverHomeAttributeValues')
      .mockResolvedValue(original)
    const view = render(AttributeDiscovery, {
      service,
      range: {},
      revision: 0,
      queries: { traces: null, logs: false, metrics: false },
      selectedAttribute: null,
      onSelectAttribute: vi.fn(),
    })
    await view.findByRole('button', { name: 'Expand retained · span values' })
    await fireEvent.click(
      view.getByRole('button', { name: 'Expand all attributes' })
    )
    await waitFor(() =>
      expect(
        view.container.querySelector('tr[data-value-identity="old"]')
      ).toBeVisible()
    )
    type Page = Awaited<ReturnType<typeof telemetryAPI.discoverAttributeKeys>>
    const pending = new Map<
      string,
      ReturnType<typeof Promise.withResolvers<Page>>
    >()
    keys.mockImplementation(options => {
      const request = Promise.withResolvers<Page>()
      pending.set(options.ownerType!, request)
      return request.promise
    })
    const replacement = Promise.withResolvers<HomeCoveragePage>()
    values.mockReturnValue(replacement.promise)
    await view.rerender({ service: { ...service, serviceName: 'next' } })
    pending.get('event')!.resolve({ items: [], truncated: false })
    await tick()
    await tick()
    expect(
      view.getByRole('button', { name: 'Collapse retained · span values' })
    ).toHaveAttribute('aria-expanded', 'true')
    expect(
      view.container.querySelector('tr[data-value-identity="old"]')
    ).toBeVisible()
    if (outcome === 'failure') {
      replacement.reject(new Error('Values unavailable'))
      pending.get('span')!.reject(new Error('Keys unavailable'))
      await waitFor(() => expect(view.getAllByRole('alert')).toHaveLength(2))
      expect(
        view.container.querySelector('tr[data-value-identity="old"]')
      ).toBeVisible()
    } else {
      pending.get('span')!.resolve({
        items: [{ key: 'retained', kind: 'string', foundOn }],
        truncated: false,
      })
      if (outcome === 'bulk')
        await fireEvent.click(
          view.getByRole('button', { name: 'Expand all attributes' })
        )
      replacement.resolve(
        homeResult([
          {
            identity: 'new',
            value: { kind: 'string', value: 'Replacement value' },
            foundOn,
            count: 2,
            kindCount: 2,
            denominator: 2,
            relativeFrequency: 1,
          },
        ])
      )
      await waitFor(() =>
        expect(
          view.container.querySelector('tr[data-value-identity="new"]')
        ).toBeInTheDocument()
      )
      expect(
        view.container.querySelector('tr[data-value-identity="old"]')
      ).toBeNull()
      const row = view.container.querySelector('tr[data-value-identity="new"]')
      if (outcome === 'bulk') expect(row).toBeVisible()
      else expect(row).toHaveAttribute('hidden')
    }
  }
)

it('retains attribute rows and disclosures through a pending or failed refresh', async () => {
  const foundOn = [{ signal: 'traces' as const, ownerType: 'span' as const }]
  const keys = vi
    .spyOn(telemetryAPI, 'discoverAttributeKeys')
    .mockImplementation(async options => ({
      items:
        options.signal === 'traces' && options.ownerType === 'span'
          ? [{ key: 'retained', kind: 'string', foundOn }]
          : [],
      truncated: false,
    }))
  vi.spyOn(telemetryAPI, 'discoverHomeAttributeValues').mockResolvedValue({
    values: [
      {
        identity: 'identity',
        valueRef: 'value',
        kind: 'string',
        preview: 'Original value',
        colorHash: 0,
        counts: { traces: 1, metrics: 0, logs: 0 },
        count: 1,
        kindCount: 1,
        denominator: 1,
        relativeFrequency: 1,
        foundOn,
      },
    ],
    kinds: [
      { kind: 'string', count: 1, counts: { traces: 1, metrics: 0, logs: 0 } },
    ],
    denominators: { traces: 1, metrics: 0, logs: 0 },
    denominator: 1,
    total: 1,
    truncated: false,
  })
  const view = render(AttributeDiscovery, {
    service,
    range: {},
    revision: 0,
    selectedAttribute: null,
    onSelectAttribute: vi.fn(),
  })
  await waitFor(() =>
    expect(
      view.getByRole('button', { name: 'Expand retained · span values' })
    ).toBeEnabled()
  )
  await fireEvent.click(
    view.getByRole('button', { name: 'Expand retained · span values' })
  )
  const toggle = view.getByRole('button', {
    name: 'Collapse retained · span values',
  })
  const row = view.container.querySelector(
    '[data-attribute-heading="retained"]'
  )
  let fail!: (reason: Error) => void
  keys.mockImplementation(
    () =>
      new Promise((_resolve, reject) => {
        fail = reject
      })
  )
  await view.rerender({ revision: 1 })
  expect(
    view.container.querySelector('[data-attribute-heading="retained"]')
  ).toBe(row)
  expect(
    view.getByRole('button', { name: 'Collapse retained · span values' })
  ).toBe(toggle)
  expect(view.queryByText('Loading attributes…')).toBeNull()
  await view.findByText('Loading attributes…')
  fail(new Error('Discovery unavailable'))
  await view.findByRole('alert')
  expect(
    view.container.querySelector('[data-attribute-heading="retained"]')
  ).toBe(row)
  expect(
    view.getByRole('button', { name: 'Collapse retained · span values' })
  ).toBe(toggle)
})

it.each([25, 14])(
  'loads %i-row server pages without skipping values and resolves full hover/Copy text only on demand',
  async rowsPerPage => {
    const foundOn = [{ signal: 'traces' as const, ownerType: 'span' as const }]
    vi.spyOn(telemetryAPI, 'discoverAttributeKeys').mockImplementation(
      async options => ({
        items:
          options.signal === 'traces' && options.ownerType === 'span'
            ? [{ key: 'deep', kind: 'map', foundOn }]
            : [],
        truncated: false,
      })
    )
    const read = vi
      .spyOn(telemetryAPI, 'discoverHomeAttributeValues')
      .mockImplementation(async options => {
        const offset = options.offset ?? 0
        return {
          values: Array.from(
            { length: Math.min(options.limit ?? 25, rowsPerPage, 64 - offset) },
            (_, i) => ({
              identity: `home:ref-${offset + i}`,
              valueRef: `home:ref-${offset + i}`,
              kind: 'map',
              preview: '{same bounded preview}',
              colorHash: 17,
              count: 1,
              counts: { traces: 1, logs: 0, metrics: 0 },
              kindCount: 64,
              denominator: 64,
              relativeFrequency: 1 / 64,
              foundOn,
            })
          ),
          kinds: [
            {
              kind: 'map',
              count: 64,
              counts: { traces: 64, logs: 0, metrics: 0 },
            },
          ],
          denominator: 64,
          denominators: { traces: 64, logs: 0, metrics: 0 },
          total: 64,
          truncated: offset + rowsPerPage < 64,
        }
      })
    const full = vi
      .spyOn(telemetryAPI, 'getHomeAttributeValue')
      .mockResolvedValue({
        kind: 'map',
        value: [
          { key: 'exact', value: { kind: 'int64', value: 9007199254740993n } },
        ],
      })
    const onSelectAttribute = vi.fn()
    const { container, getByRole } = render(AttributeDiscovery, {
      service,
      range: {},
      revision: 0,
      queries: { traces: null, logs: false, metrics: false },
      selectedAttribute: null,
      onSelectAttribute,
    })
    await waitFor(() =>
      expect(read).toHaveBeenCalledWith(
        expect.objectContaining({ limit: 0 }),
        expect.any(AbortSignal)
      )
    )
    expect(full).not.toHaveBeenCalled()
    await fireEvent.click(
      getByRole('button', { name: 'Expand deep · span values' })
    )
    await waitFor(() =>
      expect(
        container.querySelectorAll('tr[data-value-identity]')
      ).toHaveLength(rowsPerPage)
    )
    expect(full).not.toHaveBeenCalled()
    const row = container.querySelector<HTMLElement>('tr[data-value-identity]')!
    await fireEvent.click(
      row.querySelector('[data-attribute-column="count"]')!,
      {
        detail: 1,
      }
    )
    expect(onSelectAttribute).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({ key: 'deep' }),
      expect.objectContaining({ identity: 'home:ref-0' })
    )
    const inspect = within(row).getByRole('button', {
      name: 'Show span attribute: deep in details',
    })
    await fireEvent.pointerEnter(inspect)
    await waitFor(() =>
      expect(inspect).toHaveAttribute(
        'title',
        '{\n  "exact": 9007199254740993\n}'
      )
    )
    await fireEvent.click(
      within(row).getByRole('button', { name: 'Copy deep value' })
    )
    await waitFor(() =>
      expect(copyText).toHaveBeenCalledExactlyOnceWith(
        '{\n  "exact": 9007199254740993\n}'
      )
    )
    expect(onSelectAttribute).toHaveBeenCalledTimes(1)
    const next = within(container.querySelector('tbody')!).getByRole('button', {
      name: 'Next page',
    })
    await fireEvent.click(next)
    await waitFor(() =>
      expect(read).toHaveBeenCalledWith(
        expect.objectContaining({ offset: rowsPerPage, limit: 25 }),
        expect.any(AbortSignal)
      )
    )
    await waitFor(() =>
      expect(
        container.querySelector('tr[data-value-identity]')
      ).toHaveAttribute('data-value-identity', `home:ref-${rowsPerPage}`)
    )
    expect(full).toHaveBeenCalledTimes(2)
    const previous = within(container.querySelector('tbody')!).getByRole(
      'button',
      { name: 'Previous page' }
    )
    await fireEvent.click(previous)
    await waitFor(() =>
      expect(
        container.querySelector('tr[data-value-identity]')
      ).toHaveAttribute('data-value-identity', 'home:ref-0')
    )
    expect(previous).toBeDisabled()
    for (let offset = rowsPerPage; offset < 64; offset += rowsPerPage) {
      await fireEvent.click(next)
      await waitFor(() =>
        expect(
          container.querySelector('tr[data-value-identity]')
        ).toHaveAttribute('data-value-identity', `home:ref-${offset}`)
      )
    }
    expect(next).toBeDisabled()
    await fireEvent.click(previous)
    await waitFor(() =>
      expect(
        container.querySelector('tr[data-value-identity]')
      ).toHaveAttribute(
        'data-value-identity',
        `home:ref-${Math.floor(63 / rowsPerPage) * rowsPerPage - rowsPerPage}`
      )
    )
    full.mockRejectedValueOnce(
      new Error(
        'Home value is unavailable; refresh attribute values and select it again'
      )
    )
    await fireEvent.click(
      within(container.querySelector('tr[data-value-identity]')!).getByRole(
        'button',
        { name: 'Copy deep value' }
      )
    )
    await waitFor(() =>
      expect(getByRole('alert')).toHaveTextContent('refresh attribute values')
    )
    expect(copyText).toHaveBeenCalledTimes(1)
  }
)

type CoverageFixtureValue = Pick<
  HomeCoverageValue,
  | 'identity'
  | 'foundOn'
  | 'count'
  | 'kindCount'
  | 'denominator'
  | 'relativeFrequency'
> & { value: AttributeValue }

function homeResult(
  items: CoverageFixtureValue[],
  options: Partial<DiscoveryOptions> = {}
): HomeCoveragePage {
  const denominator = items[0]?.denominator ?? 0
  const denominators = { traces: denominator, metrics: 0, logs: 0 }
  const kinds = [
    ...new Map(
      items.map(item => [
        item.value.kind,
        {
          kind: item.value.kind,
          count: item.kindCount,
          counts: { traces: item.kindCount, metrics: 0, logs: 0 },
        },
      ])
    ).values(),
  ]
  const filtered = items.filter(
    item => !options.valueKind || item.value.kind === options.valueKind
  )
  return {
    values: filtered
      .slice(options.offset ?? 0, (options.offset ?? 0) + (options.limit ?? 25))
      .map(item => ({
        identity: item.identity,
        valueRef: item.identity,
        kind: item.value.kind,
        preview: attributeValueCopy(item.value),
        colorHash: 17,
        foundOn: item.foundOn,
        count: item.count,
        counts: { traces: item.count, metrics: 0, logs: 0 },
        kindCount: item.kindCount,
        denominator,
        relativeFrequency: item.relativeFrequency,
      })),
    kinds,
    denominators,
    denominator,
    total: filtered.length,
    truncated: filtered.length > 25,
  }
}

it.each([
  {
    ownerType: 'span' as const,
    stringCount: 99,
    valueCount: 99,
    denominator: 100,
    strings: ['1'],
  },
  // Three spans: one carries both strings on events, the others one each.
  {
    ownerType: 'event' as const,
    stringCount: 3,
    valueCount: 2,
    denominator: 4,
    strings: ['1', '2'],
  },
])(
  'renders and sorts distinct typed counts for $ownerType while retaining all-kind percentages',
  async ({ ownerType, stringCount, valueCount, denominator, strings }) => {
    const foundOn = [{ signal: 'traces' as const, ownerType }]
    vi.spyOn(telemetryAPI, 'discoverAttributeKeys').mockImplementation(
      async options => ({
        items:
          options.signal === 'traces' && options.ownerType === ownerType
            ? ['int64', 'string'].map(kind => ({
                key: 'attempt',
                kind,
                foundOn,
              }))
            : [],
        truncated: false,
      })
    )
    const values: CoverageFixtureValue[] = [
      ...strings.map(value => ({
        identity: JSON.stringify({ kind: 'string', value }),
        value: { kind: 'string' as const, value },
        foundOn,
        count: valueCount,
        kindCount: stringCount,
        denominator,
        relativeFrequency: valueCount / denominator,
      })),
      {
        identity: '{"kind":"int64","value":"1"}',
        value: { kind: 'int64', value: 1n },
        foundOn,
        count: 1,
        kindCount: 1,
        denominator,
        relativeFrequency: 1 / denominator,
      },
    ]
    vi.spyOn(telemetryAPI, 'discoverHomeAttributeValues').mockImplementation(
      async options => homeResult(values, options)
    )
    const { container, getByRole } = render(AttributeDiscovery, {
      service: {
        serviceName: 'checkout',
        serviceNamespace: '',
        traceCount: denominator,
        spanCount: denominator,
        errorSpanCount: 0,
        logCount: 0,
        errorLogCount: 0,
        logSeverityCounts: {
          trace: 0,
          debug: 0,
          info: 0,
          warn: 0,
          error: 0,
          fatal: 0,
          other: 0,
        },
        metricCount: 0,
        dataPointCount: 0,
        exemplarCount: 0,
        lastSeen: null,
      },
      range: {},
      revision: 0,
      queries: { traces: null, logs: false, metrics: false },
      selectedAttribute: null,
      onSelectAttribute: vi.fn(),
    })
    await waitFor(() =>
      expect(
        container.querySelectorAll('tr[data-attribute-heading]')
      ).toHaveLength(2)
    )
    const heading = (kind: string) =>
      getByRole('button', {
        name: `Show attempt · ${kind} · ${ownerType} in details`,
      }).closest('tr')!
    const stringHeading = heading('string')
    const intHeading = heading('int64')
    await waitFor(() => {
      expect(
        stringHeading.querySelector('[data-attribute-column="count"]')
      ).toHaveTextContent(new RegExp(`^${stringCount}$`))
      expect(
        intHeading.querySelector('[data-attribute-column="count"]')
      ).toHaveTextContent(/^1$/)
    })
    expect(
      within(stringHeading).getByTitle(
        `${strings.length} distinct ${strings.length === 1 ? 'value' : 'values'} · ${stringCount} spans`
      )
    ).toBeInTheDocument()
    expect(
      within(intHeading).getByTitle('1 distinct value · 1 spans')
    ).toBeInTheDocument()

    const headings = () => [
      ...container.querySelectorAll('tr[data-attribute-heading]'),
    ]
    await fireEvent.click(getByRole('button', { name: 'Sort by records' }))
    expect(headings()).toEqual([stringHeading, intHeading])
    await fireEvent.click(getByRole('button', { name: 'Sort by records' }))
    expect(headings()).toEqual([intHeading, stringHeading])

    await fireEvent.click(
      within(intHeading).getByRole('button', {
        name: `Expand attempt · ${ownerType} values`,
      })
    )
    const intValue = container.querySelector<HTMLTableRowElement>(
      `tr[data-value-identity='{"kind":"int64","value":"1"}']`
    )!
    expect(
      within(intValue).getByLabelText(
        `1 / ${denominator} records · ${(100 / denominator).toLocaleString(undefined, { maximumFractionDigits: 1 })}%`
      )
    ).toBeVisible()
    expect(
      within(intValue).getByTitle(
        `1 / ${denominator} spans · Of records carrying this attribute`
      )
    ).toBeVisible()
  }
)

it('paginates attribute names, keeps global count sorting and retains expanded values across pages', async () => {
  const foundOn = [{ signal: 'traces' as const, ownerType: 'span' as const }]
  const entries = Array.from({ length: 60 }, (_, index) => ({
    key: `key-${String(index).padStart(3, '0')}`,
    kind: 'string',
    foundOn,
  }))
  vi.spyOn(telemetryAPI, 'discoverAttributeKeys').mockImplementation(
    async options => ({
      items:
        options.signal === 'traces' && options.ownerType === 'span'
          ? entries.filter(
              entry => !options.term || entry.key.includes(options.term)
            )
          : [],
      truncated: false,
    })
  )
  const values = vi
    .spyOn(telemetryAPI, 'discoverHomeAttributeValues')
    .mockImplementation(async options => {
      const count = Number(options.key?.slice(4)) + 1
      return homeResult(
        [
          {
            identity: '{"kind":"string","value":"received"}',
            value: { kind: 'string', value: 'received' },
            foundOn,
            count,
            kindCount: count,
            denominator: 100,
            relativeFrequency: count / 100,
          },
        ],
        options
      )
    })
  const onSelectAttribute = vi.fn()
  const { container, getByRole, rerender } = render(AttributeDiscovery, {
    service,
    range: {},
    revision: 0,
    queries: { traces: null, logs: false, metrics: false },
    selectedAttribute: null,
    onSelectAttribute,
  })
  // Keep control queries out of the attribute/value rows: computing every
  // button's accessible name repeatedly is expensive for the expanded table.
  const controls = within(getByRole('toolbar', { name: 'Attribute controls' }))
  const footer = within(container.querySelector<HTMLElement>('.signal-footer')!)
  const headings = () => [
    ...container.querySelectorAll<HTMLTableRowElement>(
      'tr[data-attribute-heading]'
    ),
  ]
  await waitFor(() => expect(headings()).toHaveLength(25))
  await waitFor(() => expect(values).toHaveBeenCalledTimes(60))
  await tick()
  const firstHeading = headings()[0]
  await fireEvent.click(
    firstHeading.querySelector('[data-attribute-column="count"]')!,
    { detail: 1 }
  )
  expect(onSelectAttribute).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({ key: 'key-000' })
  )
  onSelectAttribute.mockClear()
  await fireEvent.click(
    within(firstHeading).getByRole('button', {
      name: 'Expand key-000 · span values',
    })
  )
  expect(
    container.querySelector('tr[data-attribute-key="key-000"]')
  ).not.toHaveAttribute('hidden')
  await fireEvent.click(
    container.querySelector(
      'tr[data-attribute-key="key-000"] [data-attribute-column="count"]'
    )!,
    { detail: 1 }
  )
  expect(onSelectAttribute).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({ key: 'key-000' }),
    expect.objectContaining({ kind: 'string', preview: 'received' })
  )
  onSelectAttribute.mockClear()
  await fireEvent.click(footer.getByRole('button', { name: 'Next page' }))
  expect(headings()[0]).toHaveAttribute('data-attribute-heading', 'key-025')
  expect(container.querySelector('tr[data-attribute-key="key-000"]')).toBeNull()
  await fireEvent.click(footer.getByRole('button', { name: 'Previous page' }))
  expect(
    container.querySelector('tr[data-attribute-key="key-000"]')
  ).not.toHaveAttribute('hidden')
  expect(values).toHaveBeenCalledWith(
    expect.objectContaining({ key: 'key-000', limit: 25 }),
    expect.any(AbortSignal)
  )
  expect(onSelectAttribute).not.toHaveBeenCalled()
  await fireEvent.click(
    controls.getByRole('button', { name: 'Expand all attributes' })
  )
  expect(
    container.querySelectorAll('tr[data-attribute-key]:not([hidden])')
  ).toHaveLength(25)
  await fireEvent.click(footer.getByRole('button', { name: 'Next page' }))
  expect(
    container.querySelectorAll('tr[data-attribute-key]:not([hidden])')
  ).toHaveLength(25)
  await fireEvent.click(
    within(headings()[0]).getByRole('button', {
      name: 'Collapse key-025 · span values',
    })
  )
  expect(container.querySelector('tr[data-attribute-key="key-025"]')).toBeNull()
  await fireEvent.click(footer.getByRole('button', { name: 'Previous page' }))
  expect(
    container.querySelectorAll('tr[data-attribute-key]:not([hidden])')
  ).toHaveLength(25)
  await fireEvent.click(footer.getByRole('button', { name: 'Next page' }))
  expect(container.querySelector('tr[data-attribute-key="key-025"]')).toBeNull()
  await fireEvent.click(
    controls.getByRole('button', { name: 'Expand all attributes' })
  )
  expect(
    container.querySelector('tr[data-attribute-key="key-025"]')
  ).not.toHaveAttribute('hidden')
  await fireEvent.click(
    controls.getByRole('button', { name: 'Collapse all attributes' })
  )
  await fireEvent.click(footer.getByRole('button', { name: 'Previous page' }))
  expect(
    container.querySelectorAll('tr[data-attribute-key]:not([hidden])')
  ).toHaveLength(0)
  expect(values).toHaveBeenCalledWith(
    expect.objectContaining({ key: 'key-025', limit: 25 }),
    expect.any(AbortSignal)
  )
  expect(onSelectAttribute).not.toHaveBeenCalled()
  await fireEvent.click(
    within(container.querySelector('thead')!).getByRole('button', {
      name: 'Sort by records',
    })
  )
  expect(headings()[0]).toHaveAttribute('data-attribute-heading', 'key-059')
  const size = footer.getByRole('combobox', { name: 'Attributes per page' })
  await fireEvent.change(size, { target: { value: '50' } })
  expect(headings()).toHaveLength(50)
  await fireEvent.change(size, { target: { value: '100' } })
  expect(headings()).toHaveLength(60)
  expect(localStorage.getItem('home-attributes-page-size')).toBe('100')
  await rerender({ filter: 'key-059' })
  await waitFor(() => expect(headings()).toHaveLength(1))
  expect(headings()[0]).toHaveAttribute('data-attribute-heading', 'key-059')
})

it('applies the latest bulk disclosure to values that finish loading later', async () => {
  const foundOn = [{ signal: 'traces' as const, ownerType: 'span' as const }]
  vi.spyOn(telemetryAPI, 'discoverAttributeKeys').mockImplementation(
    async options => ({
      items:
        options.signal === 'traces' && options.ownerType === 'span'
          ? ['early', 'late'].map(key => ({ key, kind: 'string', foundOn }))
          : [],
      truncated: false,
    })
  )
  const early =
    Promise.withResolvers<
      Awaited<ReturnType<typeof telemetryAPI.discoverHomeAttributeValues>>
    >()
  const late =
    Promise.withResolvers<
      Awaited<ReturnType<typeof telemetryAPI.discoverHomeAttributeValues>>
    >()
  vi.spyOn(telemetryAPI, 'discoverHomeAttributeValues').mockImplementation(
    options => (options.key === 'early' ? early.promise : late.promise)
  )
  const onSelectAttribute = vi.fn()
  const { container, getByRole } = render(AttributeDiscovery, {
    service,
    range: {},
    revision: 0,
    queries: { traces: null, logs: false, metrics: false },
    selectedAttribute: null,
    onSelectAttribute,
  })
  await waitFor(() =>
    expect(getByRole('button', { name: 'Expand all attributes' })).toBeEnabled()
  )
  await fireEvent.click(getByRole('button', { name: 'Expand all attributes' }))
  const result = homeResult([
    {
      identity: 'received',
      value: { kind: 'string' as const, value: 'received' },
      foundOn,
      count: 1,
      kindCount: 1,
      denominator: 1,
      relativeFrequency: 1,
    },
  ])
  early.resolve(result)
  await waitFor(() =>
    expect(
      container.querySelector('tr[data-attribute-key="early"]')
    ).toBeVisible()
  )
  await fireEvent.click(
    getByRole('button', { name: 'Collapse all attributes' })
  )
  late.resolve(result)
  await waitFor(() =>
    expect(
      container.querySelector('tr[data-attribute-key="late"]')
    ).toHaveAttribute('hidden')
  )
  expect(
    container.querySelector('tr[data-attribute-key="early"]')
  ).toHaveAttribute('hidden')
  await fireEvent.click(getByRole('button', { name: 'Expand all attributes' }))
  expect(
    container.querySelectorAll('tr[data-attribute-key]:not([hidden])')
  ).toHaveLength(2)
  expect(onSelectAttribute).not.toHaveBeenCalled()
})
