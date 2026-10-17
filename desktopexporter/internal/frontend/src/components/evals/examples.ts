import type { AssessmentDefinition, EvalAttempt, EvalRun } from './model'

// Deliberately illustrative data, not a regrading of the completed cohort.
const assessments: AssessmentDefinition[] = [
  {
    id: 'completion',
    name: 'Task completion',
    scale: 'Fail 0 · Partial 0.5 · Pass 1',
    maximum: 1,
    aggregation: 'planned',
  },
  {
    id: 'support',
    name: 'Evidence support',
    scale: 'Unsupported 0 · Mixed 0.5 · Supported 1',
    maximum: 1,
    aggregation: 'planned',
  },
  {
    id: 'export',
    name: 'Export validity',
    scale: 'Invalid 0 · Valid 1 · Only applicable exports',
    maximum: 1,
    aggregation: 'scored',
  },
]

const cases = [
  {
    id: 'checkout-failure',
    name: 'Explain the checkout failure',
    question:
      'Which operation caused the checkout request to fail, and what evidence connects it to the request?',
    answer:
      'The payment operation failed after the upstream payment provider declined the card. The provider span is a child of the payment span in the same checkout trace; both carry the decline error.',
  },
  {
    id: 'request-latency',
    name: 'Find the slow request',
    question:
      'Identify the slowest request and explain where it spent its time.',
    answer:
      'The checkout request is the slowest. The database span is its longest individual operation, so the database accounts for most of the request latency.',
  },
  {
    id: 'trace-logs',
    name: 'Correlate the trace and logs',
    question: 'Find the error logs belonging to the failed checkout trace.',
    answer:
      'The error logs mention the payment failure, but I could not verify that every returned log belongs to this trace.',
  },
  {
    id: 'trace-export',
    name: 'Export the complete trace',
    question:
      'Export the failed trace as OTLP JSON and verify that the file contains the complete trace.',
    answer:
      'Exported the trace and checked that the output parses as OTLP JSON. All eight expected span IDs are present.',
  },
  {
    id: 'sequence-types',
    name: 'Inspect sequence types',
    question: 'Which recorded sequence types are present in the dataset?',
    answer:
      'The recorded sequences include request, database and messaging operations. The grouping uses the recorded type attributes.',
  },
  {
    id: 'cpu-evidence',
    name: 'Compare CPU evidence',
    question:
      'Compare CPU usage during the incident with the preceding window. Explain what the data does and does not establish.',
    answer:
      'CPU usage increased during the incident window. This is a temporal association; the available measurements do not establish that CPU usage caused the failed checkout.',
  },
  {
    id: 'checkout-latency',
    name: 'Compare checkout latency',
    question:
      'How did checkout latency change between the baseline and incident windows?',
    answer:
      'The incident window has a higher median and p95 than the baseline. Both comparisons use the same service, route and duration unit.',
  },
  {
    id: 'service-errors',
    name: 'Locate service errors',
    question: 'Which services recorded errors during the incident?',
    answer:
      'The checkout and payment services recorded error spans during the incident window.',
  },
]

function attempts(variant: 'guided' | 'direct' | 'planned'): EvalAttempt[] {
  return cases.map((item, index) => {
    const planned = variant === 'planned' || (variant === 'direct' && index > 5)
    const error = variant === 'direct' && index === 5
    const completion = index === 1 ? 0.5 : index === 2 ? 0 : 1
    const support = index === 1 ? 0.5 : 1
    const attempt: EvalAttempt = {
      ...item,
      repetition: 1,
      answer: planned || error ? undefined : item.answer,
      state: planned ? 'Planned' : error ? 'Error' : 'Complete',
      startedAt: planned
        ? undefined
        : `2026-10-16T21:${String(30 + index * 2).padStart(2, '0')}:05.123Z`,
      seconds: planned ? undefined : 24 + index * 13,
      investigatorCost: planned || error ? undefined : 0.035 + index * 0.011,
      graderCost: planned || error ? undefined : 0.072 + index * 0.005,
      error: error
        ? 'The investigator process exited before producing an answer. This configuration is paused; the remaining attempts have not started.'
        : undefined,
      assessments:
        planned || error
          ? []
          : [
              {
                id: 'completion',
                value: completion,
                label:
                  completion === 1
                    ? 'Pass'
                    : completion === 0.5
                      ? 'Partial'
                      : 'Fail',
                evaluator: 'Completion grader',
                rubric: 'completion-v1',
                explanation:
                  index === 1
                    ? 'Identifies the slowest request and the longest span, but does not account for overlapping work. The attribution of most latency to the database is not established.'
                    : index === 2
                      ? 'The answer explicitly leaves trace correlation unverified. Matching error text alone does not fulfil the request.'
                      : 'The answer fulfils the stated criterion and cites the relevant evidence.',
              },
              {
                id: 'support',
                value: support,
                label: support === 1 ? 'Supported' : 'Mixed',
                evaluator: 'Evidence reviewer',
                rubric: 'support-example-v1',
                explanation:
                  index === 1
                    ? 'The slow request and span duration are supported. The causal claim about total latency exceeds the available evidence.'
                    : index === 2
                      ? 'The answer accurately describes its uncertainty. Incomplete task execution is separate from making unsupported claims.'
                      : 'The factual claims are supported by the supplied evidence. No additional causal claims were made.',
              },
              ...(item.id === 'trace-export'
                ? [
                    {
                      id: 'export',
                      value: 1,
                      label: 'Valid',
                      evaluator: 'OTLP file verifier',
                      rubric: 'export-v1',
                      explanation:
                        'Valid JSON, expected OTLP structure, and all eight expected spans present.',
                    },
                  ]
                : []),
            ],
      tools: planned
        ? []
        : [
            { name: 'bash', state: 'OK', message: 'Read the viewer guide.' },
            ...(index === 1
              ? [
                  {
                    name: 'bash',
                    state: 'Error' as const,
                    message:
                      'SQL binder error: column "value" must appear in the GROUP BY clause or be used in an aggregate function.',
                  },
                ]
              : []),
            {
              name: 'bash',
              state: 'OK',
              message: 'Queried the relevant telemetry.',
            },
          ],
    }
    // Stable, synthetic IDs let the optional seed script supply matching OTLP.
    if (!planned) {
      const traceID = `eeee${variant === 'guided' ? '01' : '02'}${String(index + 1).padStart(26, '0')}`
      attempt.telemetry = { traceID, spanID: 'eeee000000000001' }
      attempt.tools.forEach((tool, toolIndex) => {
        tool.telemetry = {
          traceID,
          spanID: `eeee${String(toolIndex + 2).padStart(12, '0')}`,
        }
      })
      attempt.assessments.forEach((assessment, scoreIndex) => {
        assessment.telemetry = {
          traceID,
          spanID: `eeee${String(scoreIndex + 101).padStart(12, '0')}`,
        }
      })
    }
    return attempt
  })
}

export const exampleRuns: EvalRun[] = [
  {
    id: 'guide-assisted',
    name: 'Guide-assisted agent',
    evaluation: 'Telemetry investigation',
    source: 'Illustrative example',
    state: 'Complete',
    startedAt: '2026-10-16T21:30:05.123Z',
    configuration: [
      { name: 'Agent', value: 'Guide-assisted' },
      { name: 'Prompt', value: 'investigation-v2' },
      { name: 'Repetitions', value: '1 per case' },
    ],
    assessments,
    headlineAssessment: 'completion',
    attempts: attempts('guided'),
  },
  {
    id: 'direct',
    name: 'Direct agent',
    evaluation: 'Telemetry investigation',
    source: 'Illustrative example',
    state: 'Paused',
    startedAt: '2026-10-16T20:15:12.456Z',
    configuration: [
      { name: 'Agent', value: 'Direct' },
      { name: 'Prompt', value: 'investigation-v1' },
      { name: 'Repetitions', value: '1 per case' },
    ],
    assessments,
    headlineAssessment: 'completion',
    attempts: attempts('direct'),
  },
  {
    id: 'next',
    name: 'Revised instructions',
    evaluation: 'Telemetry investigation',
    source: 'Illustrative example',
    state: 'Planned',
    configuration: [
      { name: 'Agent', value: 'Guide-assisted' },
      { name: 'Prompt', value: 'investigation-v3' },
      { name: 'Repetitions', value: '1 per case' },
    ],
    assessments,
    headlineAssessment: 'completion',
    attempts: attempts('planned'),
  },
]
