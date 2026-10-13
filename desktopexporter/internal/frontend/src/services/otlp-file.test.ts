import { describe, expect, it, vi } from 'vitest'
import { batchOTLPFile, OTLP_REQUEST_BYTES, scanOTLPFile } from './otlp-file'

function chunkedFile(text: string, chunkSize: number): Blob {
  const file = new Blob([text])
  const bytes = new TextEncoder().encode(text)
  vi.spyOn(file, 'stream').mockImplementation(() => {
    let offset = 0
    return new ReadableStream({
      pull(controller) {
        if (offset === bytes.length) {
          controller.close()
          return
        }
        controller.enqueue(bytes.slice(offset, offset + chunkSize))
        offset = Math.min(offset + chunkSize, bytes.length)
      },
    })
  })
  return file
}

describe('OTLP file scanning and batching', () => {
  it.each([1, 7, 65536])(
    'preserves byte ranges at chunk size %i, including BOM and Unicode',
    async chunkSize => {
      const trace = String.raw`{"resource":{"attributes":[{"key":"😀","value":{"stringValue":"\ud83d\ude00 é \" ] }"}}]},"scopeSpans":[{"spans":[{"startTimeUnixNano":"18446744073709551615","attributes":[{"key":"n","value":{"doubleValue":-0}},{"key":"i","value":{"intValue":9007199254740993}}]}]}]}`
      const log =
        '{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"中文"}}]}]}'
      const file = chunkedFile(
        `\uFEFF{\n"resourceSpans":[${trace}],"resourceProfiles":[],"unexpected":{}}\n{"resourceLogs":[${log}]}\n`,
        chunkSize
      )
      const plan = await scanOTLPFile(file)
      expect(file.stream).toHaveBeenCalledOnce()
      expect(plan.traces).toHaveLength(1)
      expect(plan.logs).toHaveLength(1)
      expect(plan.profiles[0].reason).toBe('Profiles support coming soon')
      expect(plan.invalid[0].reason).toContain('unexpected')
      for (const [signal, original] of [
        ['traces', trace],
        ['logs', log],
      ] as const) {
        const batches = [...batchOTLPFile(file, signal, plan[signal])]
        expect(batches).toHaveLength(1)
        const batch = batches[0]
        if (!('body' in batch)) throw new Error(batch.issue.reason)
        expect(await batch.body.text()).toBe(
          `{"${signal === 'traces' ? 'resourceSpans' : 'resourceLogs'}":[${original}]}`
        )
      }
    }
  )

  it.each([
    '{"resourceLogs":[{}]}\n{"resourceSpans":[}',
    '{"resourceLogs":[{},]}',
    '{"resourceLogs":[{"n":01}]}',
    '{"resourceLogs":[{"s":"\\q"}]}',
    'not JSON',
    '[{"resourceLogs":[]}]',
  ])('rejects invalid file syntax before returning a plan: %s', async text => {
    await expect(scanOTLPFile(chunkedFile(text, 1))).rejects.toThrow()
  })

  it('reports invalid wrappers and empty input without misclassifying profiles', async () => {
    const plan = await scanOTLPFile(
      new Blob([
        '{"resourceSpans":null,"resourceLogs":[42],"resourceProfiles":[]}',
      ])
    )
    expect(plan.invalid).toHaveLength(2)
    expect(plan.profiles).toHaveLength(1)
    expect((await scanOTLPFile(new Blob())).invalid[0].reason).toContain(
      'No OTLP'
    )
  })

  it.each(['traces', 'logs'] as const)(
    'splits %s while preserving resource/scope metadata and record order',
    async signal => {
      const key = signal === 'traces' ? 'resourceSpans' : 'resourceLogs'
      const scopes = signal === 'traces' ? 'scopeSpans' : 'scopeLogs'
      const records = signal === 'traces' ? 'spans' : 'logRecords'
      const values = Array.from({ length: 8 }, (_, i) => ({
        name: `${i}-${'x'.repeat(50)}`,
      }))
      const resource = {
        [scopes]: [
          {
            [records]: values,
            scope: { name: 'after-array' },
            schemaUrl: 'scope-schema',
          },
        ],
        resource: { attributes: [] },
        schemaUrl: 'resource-schema',
      }
      const file = new Blob([JSON.stringify({ [key]: [resource] })])
      const plan = await scanOTLPFile(file)
      const batches = [...batchOTLPFile(file, signal, plan[signal], 350)]
      expect(batches.length).toBeGreaterThan(1)
      const received: { name: string }[] = []
      for (const batch of batches) {
        if (!('body' in batch)) throw new Error(batch.issue.reason)
        expect(batch.body.size).toBeLessThanOrEqual(350)
        const group = JSON.parse(await batch.body.text())[key][0]
        expect(group.schemaUrl).toBe('resource-schema')
        expect(group.resource).toEqual({ attributes: [] })
        const scope = group[scopes][0]
        expect(scope.scope).toEqual({ name: 'after-array' })
        expect(scope.schemaUrl).toBe('scope-schema')
        received.push(...scope[records])
      }
      expect(received).toEqual(values)
    }
  )

  it.each(['gauge', 'sum', 'histogram', 'exponentialHistogram', 'summary'])(
    'splits %s datapoints without losing Metric descriptor or metadata',
    async kind => {
      const dataPoints = Array.from({ length: 6 }, (_, i) => ({
        timeUnixNano: String(i),
        asInt: '9223372036854775807',
        attributes: [{ key: 'k', value: { stringValue: '😀'.repeat(10) } }],
      }))
      const file = new Blob([
        JSON.stringify({
          resourceMetrics: [
            {
              resource: { attributes: [] },
              scopeMetrics: [
                {
                  scope: { name: 'test' },
                  metrics: [
                    {
                      name: 'test.metric',
                      unit: 'ms',
                      [kind]: { dataPoints, aggregationTemporality: 2 },
                      metadata: [{ key: 'keep', value: { boolValue: true } }],
                    },
                  ],
                },
              ],
            },
          ],
        }),
      ])
      const plan = await scanOTLPFile(file)
      const received: unknown[] = []
      for (const batch of batchOTLPFile(file, 'metrics', plan.metrics, 500)) {
        if (!('body' in batch)) throw new Error(batch.issue.reason)
        expect(batch.body.size).toBeLessThanOrEqual(500)
        const resource = JSON.parse(await batch.body.text()).resourceMetrics[0]
        const scope = resource.scopeMetrics[0]
        expect(scope.scope.name).toBe('test')
        const metric = scope.metrics[0]
        expect(metric.name).toBe('test.metric')
        expect(metric.unit).toBe('ms')
        expect(metric.metadata).toEqual([
          { key: 'keep', value: { boolValue: true } },
        ])
        expect(metric[kind].aggregationTemporality).toBe(2)
        received.push(...metric[kind].dataPoints)
      }
      expect(received).toEqual(dataPoints)
    }
  )

  it('uses exact encoded byte size including wrapper at the 20 MiB boundary', async () => {
    const start =
      '{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"'
    const end = '"}}]}]}]}'
    const text =
      start + 'x'.repeat(OTLP_REQUEST_BYTES - start.length - end.length) + end
    const file = new Blob([text])
    expect(file.size).toBe(OTLP_REQUEST_BYTES)
    const plan = await scanOTLPFile(file)
    const batches = [...batchOTLPFile(file, 'logs', plan.logs)]
    expect(batches).toHaveLength(1)
    if (!('body' in batches[0])) throw new Error('Expected batch')
    expect(batches[0].body.size).toBe(OTLP_REQUEST_BYTES)
    expect(await batches[0].body.text()).toBe(text)
    const larger = new Blob([
      start,
      'x'.repeat(OTLP_REQUEST_BYTES - start.length - end.length + 1),
      end,
    ])
    const oversized = await scanOTLPFile(larger)
    expect([...batchOTLPFile(larger, 'logs', oversized.logs)]).toEqual([
      {
        issue: expect.objectContaining({
          reason: expect.stringContaining('20 MiB'),
        }),
      },
    ])
  }, 20000)

  it('keeps later records when one record is oversized', async () => {
    const file = new Blob([
      JSON.stringify({
        resourceLogs: [
          {
            scopeLogs: [
              {
                logRecords: [
                  { body: { stringValue: 'x'.repeat(500) } },
                  { body: { stringValue: 'small' } },
                ],
              },
            ],
          },
        ],
      }),
    ])
    const plan = await scanOTLPFile(file)
    const batches = [...batchOTLPFile(file, 'logs', plan.logs, 200)]
    expect(batches.some(batch => 'issue' in batch)).toBe(true)
    const sent = batches.find(batch => 'body' in batch)
    if (!sent || !('body' in sent)) throw new Error('Expected remaining record')
    expect(
      JSON.parse(await sent.body.text()).resourceLogs[0].scopeLogs[0].logRecords
    ).toEqual([{ body: { stringValue: 'small' } }])
  })

  it('retains exact numeric lexemes inside split records', async () => {
    const record =
      '{"timeUnixNano":18446744073709551615,"body":{"doubleValue":-0.0},"attributes":[{"key":"n","value":{"intValue":9223372036854775807}}]}'
    const file = new Blob([
      `{"resourceLogs":[{"scopeLogs":[{"logRecords":[${Array(8).fill(record).join(',')}]}]}]}`,
    ])
    const plan = await scanOTLPFile(file)
    let copies = 0
    let requests = 0
    for (const batch of batchOTLPFile(file, 'logs', plan.logs, 400)) {
      if (!('body' in batch)) throw new Error(batch.issue.reason)
      copies += (await batch.body.text()).split(record).length - 1
      requests++
    }
    expect(copies).toBe(8)
    expect(requests).toBeGreaterThan(1)
  })

  it('rejects invalid UTF-8 rather than substituting replacement characters', async () => {
    const file = new Blob([
      '{"resourceLogs":[{"body":"',
      new Uint8Array([0xff, 0xff, 0xff, 0xff]),
      '"}]}',
    ])
    await expect(scanOTLPFile(file)).rejects.toThrow()
  })

  it('does not silently omit malformed collection members during splitting', async () => {
    const file = new Blob([
      `{"resourceLogs":[{"scopeLogs":[{"logRecords":[null,{"body":{"stringValue":"${'x'.repeat(200)}"}}]}]}]}`,
    ])
    const plan = await scanOTLPFile(file)
    expect(
      [...batchOTLPFile(file, 'logs', plan.logs, 100)].every(
        batch => 'issue' in batch
      )
    ).toBe(true)
  })
})
