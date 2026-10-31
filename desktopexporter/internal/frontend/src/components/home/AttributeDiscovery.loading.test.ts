// @vitest-environment jsdom
import { cleanup, fireEvent, render, within } from '@testing-library/svelte'
import { afterEach, expect, it, vi } from 'vitest'
import {
  Binary,
  Bool,
  Field,
  Float64,
  Int64,
  List,
  Map_,
  Struct,
  Table,
  Utf8,
  makeData,
  makeVector,
  vectorFromArray,
} from 'apache-arrow'
import { arrowResponse } from '@/test/arrow-fixtures'
import type {
  DiscoveryOptions,
  HomeCoveragePage,
} from '@/services/discovery-types'
import AttributeDiscovery from './AttributeDiscovery.svelte'

type Request = {
  options: DiscoveryOptions
  signal: AbortSignal
  resolve: (response: Response) => void
  reject: (reason: Error) => void
  result: (result: HomeCoveragePage) => void
}

function transport() {
  const requests: Request[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((_url: string, init: RequestInit) => {
      const body = JSON.parse(String(init.body))
      const pending = Promise.withResolvers<Response>()
      requests.push({
        options: body.params?.options ?? body.options,
        signal: init.signal as AbortSignal,
        ...pending,
        result: result =>
          pending.resolve(
            Response.json({ jsonrpc: '2.0', id: body.id, result })
          ),
      })
      return pending.promise
    })
  )
  return requests
}

function keys(names: string[], truncated = false, kind = 'string') {
  return arrowResponse(
    new Table({
      items: vectorFromArray(
        [names.map(key => ({ key, kind, foundOn: [] }))],
        new List(
          new Field(
            'item',
            new Struct([
              new Field('key', new Utf8()),
              new Field('kind', new Utf8()),
              new Field(
                'foundOn',
                new List(
                  new Field(
                    'item',
                    new Struct([
                      new Field('signal', new Utf8()),
                      new Field('ownerType', new Utf8()),
                    ])
                  )
                )
              ),
            ])
          )
        )
      ),
      truncated: vectorFromArray([truncated], new Bool()),
    })
  )
}

function coverage(
  identity: string,
  count = 3,
  denominator = 10
): HomeCoveragePage {
  const counts = { traces: count, metrics: 0, logs: 0 }
  return {
    values: [
      {
        identity,
        valueRef: identity,
        kind: 'int64',
        preview: 'bounded preview',
        colorHash: 1,
        counts,
        count,
        kindCount: count,
        denominator,
        relativeFrequency: count / denominator,
        foundOn: [{ signal: 'traces', ownerType: 'span' }],
      },
    ],
    kinds: [{ kind: 'int64', count, counts }],
    denominators: { traces: denominator, metrics: 0, logs: 0 },
    denominator,
    total: 1,
    truncated: false,
  }
}

const props = {
  service: {
    serviceName: 'A',
    serviceNamespace: '',
    traceCount: 10,
    spanCount: 10,
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
  selectedAttribute: null,
  onSelectAttribute: vi.fn(),
}

async function settle() {
  await vi.advanceTimersByTimeAsync(0)
}

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

it('starts every owner immediately and publishes byte-bounded pages beyond 100 keys without sibling barriers or page resets', async () => {
  vi.useFakeTimers()
  const requests = transport()
  const view = render(AttributeDiscovery, props)
  expect(requests).toHaveLength(13)
  expect(requests.every(request => request.options.kind === 'keys')).toBe(true)
  expect(
    new Set(
      requests.map(({ options }) => `${options.signal}/${options.ownerType}`)
    ).size
  ).toBe(13)
  const span = requests.find(({ options }) => options.ownerType === 'span')!
  span.resolve(
    keys(
      Array.from({ length: 26 }, (_, i) => `key-${String(i).padStart(3, '0')}`),
      true
    )
  )
  await settle()
  expect(
    requests.filter(({ options }) => options.kind === 'home-values')
  ).toHaveLength(26)
  expect(
    requests.find(
      ({ options }) => options.kind === 'keys' && options.offset === 26
    )
  ).toBeDefined()
  const footer = within(
    view.container.querySelector<HTMLElement>('.signal-footer')!
  )
  await fireEvent.click(footer.getByRole('button', { name: 'Next page' }))
  expect(
    view.container.querySelector('[data-attribute-heading]')
  ).toHaveAttribute('data-attribute-heading', 'key-025')
  const event = requests.find(({ options }) => options.ownerType === 'event')!
  event.resolve(keys([]))
  await settle()
  expect(
    view.container.querySelector('[data-attribute-heading]')
  ).toHaveAttribute('data-attribute-heading', 'key-025')
  requests
    .find(({ options }) => options.kind === 'keys' && options.offset === 26)!
    .resolve(
      keys(
        Array.from(
          { length: 80 },
          (_, i) => `key-${String(i + 26).padStart(3, '0')}`
        )
      )
    )
  await settle()
  expect(
    requests.filter(({ options }) => options.kind === 'home-values')
  ).toHaveLength(106)
  expect(footer.getByText('2 / 5 pages')).toBeInTheDocument()
  expect(
    view.container.querySelector('[data-attribute-heading]')
  ).toHaveAttribute('data-attribute-heading', 'key-025')
  expect(
    view.container.querySelector('[data-attribute-column="count"]')
  ).not.toHaveTextContent('0')
  view.unmount()
  expect(requests.every(request => request.signal.aborted)).toBe(true)
})

it.each(['pending', 'failed'] as const)(
  'retains the exact displayed value, typed counts, percentages and Copy target during a %s replacement',
  async outcome => {
    vi.useFakeTimers()
    const requests = transport()
    const clipboard = vi.fn(async () => {})
    vi.stubGlobal('navigator', {
      ...navigator,
      language: 'en-US',
      clipboard: { writeText: clipboard },
    })
    const view = render(AttributeDiscovery, {
      ...props,
      queries: { traces: null, metrics: false, logs: false },
    })
    requests
      .find(({ options }) => options.ownerType === 'span')!
      .resolve(keys(['exact'], false, 'int64'))
    await settle()
    const initial = requests.find(
      ({ options }) => options.kind === 'home-values'
    )!
    expect(
      view.container.querySelector(
        '[data-attribute-heading] [data-attribute-column="count"]'
      )
    ).toHaveTextContent('—')
    initial.result(coverage('home:A'))
    await settle()
    await fireEvent.click(
      view.getByRole('button', { name: 'Expand exact · span values' })
    )
    requests.at(-1)!.result(coverage('home:A'))
    await settle()
    const row = view.container.querySelector<HTMLTableRowElement>(
      'tr[data-value-identity="home:A"]'
    )!
    const toggle = view.getByRole('button', {
      name: 'Collapse exact · span values',
    })
    expect(within(row).getByLabelText('3 / 10 records · 30%')).toBeVisible()
    expect(row).toHaveTextContent('bounded preview')
    expect(
      requests.filter(({ options }) => options.kind === 'home-value')
    ).toHaveLength(0)
    await view.rerender({ service: { ...props.service, serviceName: 'B' } })
    await settle()
    expect(
      view.getByRole('button', { name: 'Collapse exact · span values' })
    ).toBe(toggle)
    expect(
      view.container.querySelector('tr[data-value-identity="home:A"]')
    ).toBe(row)
    expect(within(row).getByLabelText('3 / 10 records · 30%')).toBeVisible()
    const replacement = requests.findLast(
      ({ options }) => options.kind === 'home-values'
    )!
    if (outcome === 'failed') {
      replacement.reject(new Error('Values unavailable'))
      await settle()
      expect(view.getByRole('alert')).toHaveTextContent('Values unavailable')
    }
    await fireEvent.click(
      within(row).getByRole('button', { name: 'Copy exact value' })
    )
    const copy = requests.at(-1)!
    expect(copy.options).toEqual({ kind: 'home-value', valueRef: 'home:A' })
    copy.resolve(
      arrowResponse(
        new Table({ value: vectorFromArray([9007199254740993n], new Int64()) })
      )
    )
    await settle()
    expect(clipboard).toHaveBeenCalledExactlyOnceWith('9007199254740993')
  }
)

it('resets the shared 200ms grace on A → B → A and rejects obsolete key/value successes and failures', async () => {
  vi.useFakeTimers()
  const requests = transport()
  const view = render(AttributeDiscovery, {
    ...props,
    queries: { traces: null, metrics: false, logs: false },
  })
  const oldKeys = requests.slice()
  oldKeys
    .find(({ options }) => options.ownerType === 'span')!
    .resolve(keys(['stable'], false, 'int64'))
  await settle()
  const oldValue = requests.at(-1)!
  await vi.advanceTimersByTimeAsync(199)
  expect(view.queryByText('Loading attributes…')).toBeNull()
  expect(view.queryByText('Loading values…')).toBeNull()
  await vi.advanceTimersByTimeAsync(1)
  expect(view.getByText('Loading attributes…')).toBeInTheDocument()
  expect(view.getByText('Loading values…')).toBeInTheDocument()
  await view.rerender({ service: { ...props.service, serviceName: 'B' } })
  const oldB = requests.findLast(
    ({ options }) => options.kind === 'home-values'
  )!
  expect(oldValue.signal.aborted).toBe(true)
  expect(view.queryByText('Loading attributes…')).toBeNull()
  expect(view.queryByText('Loading values…')).toBeNull()
  await vi.advanceTimersByTimeAsync(199)
  expect(view.queryByText('Loading attributes…')).toBeNull()
  await view.rerender({ service: props.service })
  const current = requests.findLast(
    ({ options }) => options.kind === 'home-values'
  )!
  expect(oldB.signal.aborted).toBe(true)
  oldValue.result(coverage('obsolete-A'))
  oldB.reject(new Error('Obsolete B failure'))
  oldKeys
    .find(({ options }) => options.ownerType === 'event')!
    .resolve(keys(['obsolete-key']))
  oldKeys
    .find(({ options }) => options.ownerType === 'resource')!
    .reject(new Error('Obsolete key failure'))
  await settle()
  expect(view.queryByRole('alert')).toBeNull()
  expect(view.container.querySelector('[data-value-identity]')).toBeNull()
  expect(
    view.container.querySelector('[data-attribute-heading="obsolete-key"]')
  ).toBeNull()
  await vi.advanceTimersByTimeAsync(199)
  expect(view.queryByText('Loading values…')).toBeNull()
  expect(view.queryByText('Loading attributes…')).toBeNull()
  current.result(coverage('current-A'))
  for (const request of requests.filter(
    request => !request.signal.aborted && request.options.kind === 'keys'
  ))
    request.resolve(
      keys(
        request.options.ownerType === 'span' ? ['stable'] : [],
        false,
        'int64'
      )
    )
  await settle()
  await vi.advanceTimersByTimeAsync(1000)
  expect(view.queryByText('Loading attributes…')).toBeNull()
  expect(view.queryByText('Loading values…')).toBeNull()
  expect(
    view.container.querySelector('[data-value-identity="current-A"]')
  ).toBeInTheDocument()
  await view.rerender({ revision: 1 })
  const unmountedValue = requests.findLast(
    ({ options }) => options.kind === 'home-values'
  )!
  const unmountedKeys = requests.filter(
    request => !request.signal.aborted && request.options.kind === 'keys'
  )
  view.unmount()
  expect(unmountedValue.signal.aborted).toBe(true)
  expect(unmountedKeys.every(request => request.signal.aborted)).toBe(true)
  unmountedValue.reject(new Error('Unmounted value failure'))
  unmountedKeys[0]!.resolve(keys(['unmounted']))
  unmountedKeys[1]!.reject(new Error('Unmounted key failure'))
  await vi.advanceTimersByTimeAsync(1000)
  expect(view.container).toBeEmptyDOMElement()
})

it.each([
  { order: 'page-first', outcome: 'pending' },
  { order: 'page-first', outcome: 'failed' },
  { order: 'replacement-first', outcome: 'pending' },
  { order: 'replacement-first', outcome: 'failed' },
] as const)(
  'pages the displayed value scope with $outcome replacement: $order',
  async ({ order, outcome }) => {
    vi.useFakeTimers()
    const requests = transport()
    const view = render(AttributeDiscovery, {
      ...props,
      queries: { traces: null, metrics: false, logs: false },
    })
    requests
      .find(({ options }) => options.ownerType === 'span')!
      .resolve(keys(['paged'], false, 'int64'))
    await settle()
    const first = { ...coverage('home:A'), total: 26, truncated: true }
    requests.at(-1)!.result(first)
    await settle()
    await fireEvent.click(
      view.getByRole('button', { name: 'Expand paged · span values' })
    )
    requests.at(-1)!.result(first)
    await settle()
    const navigation = within(view.container.querySelector('tbody')!).getByRole(
      'group',
      { name: 'page navigation' }
    )
    expect(
      within(navigation).getByRole('button', { name: 'Next page' })
    ).toBeEnabled()
    if (order === 'page-first')
      await fireEvent.click(
        within(navigation).getByRole('button', { name: 'Next page' })
      )
    const initialPage = order === 'page-first' ? requests.at(-1) : undefined
    await view.rerender({ service: { ...props.service, serviceName: 'B' } })
    const replacement = requests.findLast(
      ({ options }) => options.kind === 'home-values'
    )!
    if (outcome === 'failed') replacement.reject(new Error('B unavailable'))
    await settle()
    if (order === 'replacement-first')
      await fireEvent.click(
        within(navigation).getByRole('button', { name: 'Next page' })
      )
    const retainedPage = initialPage ?? requests.at(-1)!
    expect(retainedPage.signal.aborted).toBe(false)
    expect(retainedPage.options).toMatchObject({
      kind: 'home-values',
      serviceName: 'A',
      offset: 1,
      limit: 25,
    })
    expect(replacement.signal.aborted).toBe(false)
    retainedPage.result(coverage('home:A-next'))
    await settle()
    expect(
      view.container.querySelector('[data-value-identity="home:A-next"]')
    ).toBeVisible()
    expect(
      view.getByRole('button', { name: 'Collapse paged · span values' })
    ).toHaveAttribute('aria-expanded', 'true')
    if (outcome === 'pending') {
      replacement.result(coverage('home:B'))
      await settle()
      expect(
        view.container.querySelector('[data-value-identity="home:A-next"]')
      ).toBeNull()
      expect(
        view.container.querySelector('[data-value-identity="home:B"]')
      ).toBeInTheDocument()
    }
  }
)

it.each(
  (['page-first', 'replacement-first'] as const).flatMap(order =>
    (
      [
        { ending: 'replacement', outcome: 'success' },
        { ending: 'replacement', outcome: 'failure' },
        { ending: 'unmount', outcome: 'success' },
        { ending: 'unmount', outcome: 'failure' },
      ] as const
    ).map(ending => ({ order, ...ending }))
  )
)(
  'cancels obsolete retained-page $outcome after $ending: $order',
  async ({ order, ending, outcome }) => {
    vi.useFakeTimers()
    const requests = transport()
    const view = render(AttributeDiscovery, {
      ...props,
      queries: { traces: null, metrics: false, logs: false },
    })
    requests
      .find(({ options }) => options.ownerType === 'span')!
      .resolve(keys(['paged'], false, 'int64'))
    await settle()
    const first = { ...coverage('home:A'), total: 26, truncated: true }
    requests.at(-1)!.result(first)
    await settle()
    await fireEvent.click(
      view.getByRole('button', { name: 'Expand paged · span values' })
    )
    requests.at(-1)!.result(first)
    await settle()
    const next = within(view.container.querySelector('tbody')!).getByRole(
      'button',
      {
        name: 'Next page',
      }
    )
    if (order === 'page-first') await fireEvent.click(next)
    const initialPage = order === 'page-first' ? requests.at(-1) : undefined
    await view.rerender({ service: { ...props.service, serviceName: 'B' } })
    const replacement = requests.findLast(
      ({ options }) => options.kind === 'home-values'
    )!
    if (order === 'replacement-first') await fireEvent.click(next)
    const oldPage = initialPage ?? requests.at(-1)!
    expect(oldPage.signal.aborted).toBe(false)
    if (ending === 'unmount') view.unmount()
    else {
      replacement.result(coverage('home:B'))
      await settle()
    }
    expect(oldPage.signal.aborted).toBe(true)
    if (outcome === 'success') oldPage.result(coverage('home:obsolete-page'))
    else oldPage.reject(new Error('Obsolete page failure'))
    await settle()
    if (ending === 'unmount') expect(view.container).toBeEmptyDOMElement()
    else
      expect(
        view.container.querySelector('[data-value-identity="home:B"]')
      ).toBeInTheDocument()
    expect(
      view.container.querySelector('[data-value-identity="home:obsolete-page"]')
    ).toBeNull()
    expect(view.queryByRole('alert')).toBeNull()
  }
)

it('keeps independently completed values and unknown counts, and only replaces a successful empty owner', async () => {
  vi.useFakeTimers()
  const requests = transport()
  const view = render(AttributeDiscovery, {
    ...props,
    queries: { traces: null, metrics: false, logs: false },
  })
  requests
    .find(({ options }) => options.ownerType === 'span')!
    .resolve(keys(['ready'], false, 'int64'))
  requests
    .find(({ options }) => options.ownerType === 'event')!
    .resolve(keys(['unknown'], false, 'int64'))
  await settle()
  const ready = requests.find(
    ({ options }) => options.kind === 'home-values' && options.key === 'ready'
  )!
  const unknown = requests.find(
    ({ options }) => options.kind === 'home-values' && options.key === 'unknown'
  )!
  ready.result(coverage('home:ready'))
  unknown.reject(new Error('Unknown values'))
  await settle()
  const readyHeading = view.container.querySelector(
    '[data-attribute-heading="ready"]'
  )!
  const unknownHeading = view.container.querySelector(
    '[data-attribute-heading="unknown"]'
  )!
  expect(
    readyHeading.querySelector('[data-attribute-column="count"]')
  ).toHaveTextContent(/^3$/)
  expect(
    unknownHeading.querySelector('[data-attribute-column="count"]')
  ).toHaveTextContent('—')
  expect(
    view.getByRole('button', { name: 'Expand unknown · event values' })
  ).toBeEnabled()
  await fireEvent.click(
    view.getByRole('button', { name: 'Expand unknown · event values' })
  )
  expect(requests.at(-1)!.options).toMatchObject({
    kind: 'home-values',
    key: 'unknown',
    limit: 25,
  })
  await view.rerender({ revision: 1 })
  const freshReady = requests.findLast(
    ({ options }) => options.kind === 'home-values' && options.key === 'ready'
  )!
  const freshUnknown = requests.findLast(
    ({ options }) => options.kind === 'home-values' && options.key === 'unknown'
  )!
  freshReady.reject(new Error('Ready unavailable'))
  freshUnknown.result({
    values: [],
    kinds: [],
    denominators: { traces: 0, metrics: 0, logs: 0 },
    denominator: 0,
    total: 0,
    truncated: false,
  })
  await settle()
  expect(
    readyHeading.querySelector('[data-attribute-column="count"]')
  ).toHaveTextContent(/^3$/)
  expect(
    unknownHeading.querySelector('[data-attribute-column="count"]')
  ).toHaveTextContent(/^0$/)
  expect(
    view.queryByRole('button', { name: 'Expand unknown · event values' })
  ).toBeNull()
  requests
    .findLast(
      ({ options }) => options.kind === 'keys' && options.ownerType === 'span'
    )!
    .resolve(keys([]))
  await settle()
  expect(
    view.container.querySelector('[data-attribute-heading="ready"]')
  ).toBeNull()
  expect(
    view.container.querySelector('[data-attribute-heading="unknown"]')
  ).toBe(unknownHeading)
})

it('resets disclosures for a changed time scope only when that row receives its replacement', async () => {
  vi.useFakeTimers()
  const requests = transport()
  const view = render(AttributeDiscovery, {
    ...props,
    queries: { traces: null, metrics: false, logs: false },
  })
  requests
    .find(({ options }) => options.ownerType === 'span')!
    .resolve(keys(['scoped'], false, 'int64'))
  await settle()
  requests.at(-1)!.result(coverage('home:old-range'))
  await settle()
  await fireEvent.click(
    view.getByRole('button', { name: 'Expand all attributes' })
  )
  requests.at(-1)!.result(coverage('home:old-range'))
  await settle()
  await view.rerender({ range: { startTime: '10', endTime: '20' } })
  const replacement = requests.findLast(
    ({ options }) => options.kind === 'home-values'
  )!
  expect(replacement.options).toMatchObject({ startTime: '10', endTime: '20' })
  expect(
    view.container.querySelector('[data-value-identity="home:old-range"]')
  ).toBeVisible()
  requests
    .findLast(
      ({ options }) => options.kind === 'keys' && options.ownerType === 'event'
    )!
    .resolve(keys([]))
  await settle()
  expect(
    view.container.querySelector('[data-value-identity="home:old-range"]')
  ).toBeVisible()
  replacement.result(coverage('home:new-range'))
  await settle()
  expect(
    view.container.querySelector('[data-value-identity="home:old-range"]')
  ).toBeNull()
  expect(
    view.container.querySelector('[data-value-identity="home:new-range"]')
  ).toHaveAttribute('hidden')
})

const exactCases = [
  {
    kind: 'string',
    table: () =>
      new Table({
        value: vectorFromArray(['{"native":"string"}\\n雪'], new Utf8()),
      }),
    text: '{"native":"string"}\\n雪',
  },
  {
    kind: 'bool',
    table: () => new Table({ value: vectorFromArray([false], new Bool()) }),
    text: 'false',
  },
  {
    kind: 'empty',
    table: () => new Table({ value: vectorFromArray([null], new Utf8()) }),
    text: 'null',
  },
  {
    kind: 'int64',
    table: () =>
      new Table({
        value: vectorFromArray([9223372036854775807n], new Int64()),
      }),
    text: '9223372036854775807',
  },
  {
    kind: 'double',
    table: () => new Table({ value: vectorFromArray([-0], new Float64()) }),
    text: '-0',
  },
  {
    kind: 'double',
    table: () =>
      new Table({ value: vectorFromArray([Infinity], new Float64()) }),
    text: '"Infinity"',
  },
  {
    kind: 'double',
    table: () => new Table({ value: vectorFromArray([NaN], new Float64()) }),
    text: '"NaN"',
  },
  {
    kind: 'bytes',
    table: () =>
      new Table({
        value: vectorFromArray([new Uint8Array([0, 255, 128])], new Binary()),
      }),
    text: 'AP+A',
  },
  {
    kind: 'array',
    table: () =>
      new Table({
        value: vectorFromArray(
          [Array.from({ length: 30 }, (_, i) => 9007199254740993n + BigInt(i))],
          new List(new Field('item', new Int64()))
        ),
      }),
    text:
      '[\n' +
      Array.from(
        { length: 30 },
        (_, i) => '  ' + (9007199254740993n + BigInt(i))
      ).join(',\n') +
      '\n]',
  },
  {
    kind: 'map',
    table: () => exactMap(),
    text: '{\n  "__proto__": 9007199254740993,\n  "duplicate": -9223372036854775808,\n  "duplicate": 9223372036854775807\n}',
  },
] as const

it.each(exactCases)(
  'copies complete received $kind through its exact reference only on demand: $text',
  async ({ kind, table, text }) => {
    vi.useFakeTimers()
    const requests = transport()
    const clipboard = vi.fn(async () => {})
    vi.stubGlobal('navigator', {
      ...navigator,
      language: 'en-US',
      clipboard: { writeText: clipboard },
    })
    const view = render(AttributeDiscovery, {
      ...props,
      queries: { traces: null, metrics: false, logs: false },
    })
    requests
      .find(({ options }) => options.ownerType === 'span')!
      .resolve(keys(['native'], false, kind))
    await settle()
    const result = coverage('home:native')
    result.values[0].kind = kind
    result.kinds[0].kind = kind
    requests.at(-1)!.result(result)
    await settle()
    await fireEvent.click(
      view.getByRole('button', { name: 'Expand native · span values' })
    )
    requests.at(-1)!.result(result)
    await settle()
    expect(
      requests.filter(({ options }) => options.kind === 'home-value')
    ).toHaveLength(0)
    await fireEvent.click(
      view.getByRole('button', { name: 'Copy native value' })
    )
    expect(requests.at(-1)!.options).toEqual({
      kind: 'home-value',
      valueRef: 'home:native',
    })
    requests.at(-1)!.resolve(arrowResponse(table()))
    await settle()
    expect(clipboard).toHaveBeenCalledExactlyOnceWith(text)
  }
)

function exactMap() {
  const entries = new Struct([
    new Field('key', new Utf8()),
    new Field('value', new Int64()),
  ])
  return new Table({
    value: makeVector(
      makeData({
        type: new Map_(new Field('entries', entries, false)),
        length: 1,
        valueOffsets: new Int32Array([0, 3]),
        child: makeData({
          type: entries,
          length: 3,
          children: [
            vectorFromArray(['__proto__', 'duplicate', 'duplicate'], new Utf8())
              .data[0]!,
            vectorFromArray(
              [9007199254740993n, -9223372036854775808n, 9223372036854775807n],
              new Int64()
            ).data[0]!,
          ],
        }),
      })
    ),
  })
}

it.each([
  { ending: 'supersession', outcome: 'success' },
  { ending: 'supersession', outcome: 'failure' },
  { ending: 'unmount', outcome: 'success' },
  { ending: 'unmount', outcome: 'failure' },
] as const)(
  'rejects stale exact Copy $outcome after $ending',
  async ({ ending, outcome }) => {
    vi.useFakeTimers()
    const requests = transport()
    const clipboard = vi.fn(async () => {})
    vi.stubGlobal('navigator', {
      ...navigator,
      language: 'en-US',
      clipboard: { writeText: clipboard },
    })
    const view = render(AttributeDiscovery, {
      ...props,
      queries: { traces: null, metrics: false, logs: false },
    })
    requests
      .find(({ options }) => options.ownerType === 'span')!
      .resolve(keys(['owned'], false, 'int64'))
    await settle()
    requests.at(-1)!.result(coverage('home:old'))
    await settle()
    await fireEvent.click(
      view.getByRole('button', { name: 'Expand owned · span values' })
    )
    requests.at(-1)!.result(coverage('home:old'))
    await settle()
    await fireEvent.click(
      view.getByRole('button', { name: 'Copy owned value' })
    )
    const oldCopy = requests.at(-1)!
    await view.rerender({ revision: 1 })
    const replacement = requests.findLast(
      ({ options }) => options.kind === 'home-values'
    )!
    expect(oldCopy.signal.aborted).toBe(false)
    if (ending === 'unmount') view.unmount()
    else {
      replacement.result(coverage('home:new'))
      await settle()
    }
    expect(oldCopy.signal.aborted).toBe(true)
    if (outcome === 'success')
      oldCopy.resolve(
        arrowResponse(
          new Table({
            value: vectorFromArray([9007199254740993n], new Int64()),
          })
        )
      )
    else oldCopy.reject(new Error('Obsolete exact value failure'))
    await settle()
    expect(clipboard).not.toHaveBeenCalled()
    expect(view.queryByRole('alert')).toBeNull()
    if (ending === 'unmount') expect(view.container).toBeEmptyDOMElement()
  }
)
