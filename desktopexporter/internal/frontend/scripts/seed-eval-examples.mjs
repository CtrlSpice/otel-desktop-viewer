// Add synthetic telemetry for the Evals UI examples to an explicit OTLP endpoint.
import { exampleRuns } from '../src/components/evals/examples.ts'

const endpoint = process.argv[2]
if (!endpoint)
  throw new Error(
    'Usage: node scripts/seed-eval-examples.mjs http://127.0.0.1:4318'
  )

const spans = []
for (const run of exampleRuns) {
  for (const attempt of run.attempts) {
    if (!attempt.telemetry || !attempt.startedAt) continue
    const start = BigInt(Date.parse(attempt.startedAt)) * 1_000_000n
    const end = start + BigInt(attempt.seconds) * 1_000_000_000n
    const root = {
      traceId: attempt.telemetry.traceID,
      spanId: attempt.telemetry.spanID,
      name: attempt.name,
      kind: 1,
      startTimeUnixNano: String(start),
      endTimeUnixNano: String(end),
      status: { code: attempt.error ? 2 : 1, message: attempt.error ?? '' },
      attributes: [
        { key: 'eval.example', value: { boolValue: true } },
        { key: 'eval.run', value: { stringValue: run.id } },
        { key: 'eval.case', value: { stringValue: attempt.id } },
      ],
    }
    spans.push(root)
    attempt.tools.forEach((tool, index) => {
      spans.push({
        ...root,
        spanId: tool.telemetry.spanID,
        parentSpanId: root.spanId,
        name: tool.name,
        startTimeUnixNano: String(start + BigInt(index + 1) * 1_000_000_000n),
        endTimeUnixNano: String(start + BigInt(index + 2) * 1_000_000_000n),
        status: {
          code: tool.state === 'Error' ? 2 : 1,
          message: tool.state === 'Error' ? tool.message : '',
        },
        attributes: [
          ...root.attributes,
          { key: 'tool.output', value: { stringValue: tool.message } },
        ],
      })
    })
    attempt.assessments.forEach((assessment, index) => {
      spans.push({
        ...root,
        spanId: assessment.telemetry.spanID,
        parentSpanId: root.spanId,
        name: assessment.evaluator,
        startTimeUnixNano: String(end - BigInt(index + 2) * 1_000_000_000n),
        endTimeUnixNano: String(end - BigInt(index + 1) * 1_000_000_000n),
        status: { code: 1 },
        attributes: [
          ...root.attributes,
          { key: 'eval.assessment', value: { stringValue: assessment.id } },
          {
            key: 'eval.explanation',
            value: { stringValue: assessment.explanation },
          },
          { key: 'eval.score', value: { doubleValue: assessment.value } },
        ],
      })
    })
  }
}

const response = await fetch(`${endpoint.replace(/\/$/, '')}/v1/traces`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    resourceSpans: [
      {
        resource: {
          attributes: [
            { key: 'service.name', value: { stringValue: 'eval-ui-examples' } },
          ],
        },
        scopeSpans: [{ scope: { name: 'eval-ui-examples' }, spans }],
      },
    ],
  }),
})
const result = await response.json()
if (!response.ok || result.partialSuccess?.rejectedSpans) {
  throw new Error(`Example telemetry import failed: ${JSON.stringify(result)}`)
}
console.log(
  `Loaded ${spans.length} synthetic spans. No evaluation was executed.`
)
