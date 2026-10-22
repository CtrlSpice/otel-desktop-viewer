import { describe, expect, it } from 'vitest'
import type { LogData, SpanNode, TraceData } from '@/types/api-types'
import {
  evalSpanEvidence,
  toolEvidenceSpans,
  toolLogOwners,
  uniqueToolCallID,
} from './eval-span-evidence'
import { exampleExperiments } from './examples'
import { buildEvalOutline, visibleOutlineRows } from './eval-outline'
import { groupResults } from './group-results'
import { evalToolCounts } from './model'
import { evalEventTools } from './eval-tool-events'
import { evalTimelineRecords } from './eval-timeline'
import {
  evalExecutionRows,
  evalTraceReferences,
  indexEvalTraces,
  referenceKey,
  visibleExecutionRows,
  projectEvalTraceEvidence,
  resolveEvalReference,
} from './eval-traces'

const traceID = 'a'.repeat(32)
const ref = (id: number, trace = traceID) => ({
  traceID: trace,
  spanID: id.toString(16).padStart(16, '0'),
})
function node(
  id: number,
  parent: number | null,
  depth: number,
  error = false
): SpanNode {
  return {
    depth,
    matched: true,
    spanData: {
      ...ref(id),
      parentSpanID: parent === null ? null : ref(parent).spanID,
      name: `span ${id}`,
      traceState: '',
      flags: 0,
      kind: 'Internal',
      kindCode: 1,
      startTime: 1700000000000000001n + BigInt(id),
      endTime: 1700000000000000010n + BigInt(id),
      attributes: [],
      events: [],
      links: [],
      droppedAttributesCount: 0,
      droppedEventsCount: 0,
      droppedLinksCount: 0,
      statusCodeValue: error ? 2 : 0,
      statusCode: error ? 'Error' : 'Unset',
      statusMessage: '',
      resource: { attributes: [], droppedAttributesCount: 0 },
      scope: {
        name: '',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
      },
    },
  }
}

describe('referenced eval hierarchy', () => {
  it('counts correlated tool logs only on their owning span and keeps genuine span events intact', () => {
    const root = node(1, null, 0)
    const parent = node(2, 1, 1)
    parent.spanData.events = [
      {
        name: 'genuine-event',
        timestamp: parent.spanData.startTime,
        attributes: [],
        droppedAttributesCount: 0,
      },
    ]
    const log: LogData = {
      ...ref(2),
      logRef: 'received-log',
      eventName: 'tool.execution',
      timestamp: parent.spanData.endTime,
      observedTimestamp: parent.spanData.endTime + 10n,
      body: { kind: 'string', value: 'shell' },
      severityNumber: 0,
      severityText: '',
      flags: 0,
      resource: parent.spanData.resource,
      scope: parent.spanData.scope,
      droppedAttributesCount: 0,
      attributes: [
        {
          key: 'gen_ai.tool.call.id',
          value: { kind: 'string', value: 'call' },
        },
        { key: 'gen_ai.tool.name', value: { kind: 'string', value: 'shell' } },
        { key: 'tool.is_error', value: { kind: 'bool', value: true } },
        {
          key: 'tool.output',
          value: { kind: 'string', value: 'exact\nerror' },
        },
        {
          key: 'eval.evidence.source',
          value: { kind: 'string', value: 'OpenCode' },
        },
        {
          key: 'eval.evidence.origin',
          value: { kind: 'string', value: 'runtime-records' },
        },
        {
          key: 'opencode.event.timestamp.source',
          value: { kind: 'string', value: 'tool.time.completed' },
        },
      ],
    }
    const source = {
      ...exampleExperiments[0],
      telemetry: undefined,
      results: [
        {
          ...exampleExperiments[0].results[0],
          telemetry: { traceID },
          resultTelemetry: undefined,
          tools: [],
          toolsKnown: false,
        },
      ],
    }
    const index = indexEvalTraces([
      { traceID, spans: [root, parent], unplacedSpanCount: 0 },
    ])
    const experiment = projectEvalTraceEvidence(source, index, [log])
    expect(evalToolCounts(experiment.results[0])).toEqual({
      total: 1,
      failed: 1,
    })
    const rows = buildEvalOutline(
      groupResults(experiment.results, experiment.results),
      experiment,
      index
    )
    expect(
      rows.find(row => row.reference?.spanID === ref(2).spanID)?.toolCounts
    ).toEqual({ total: 1, failed: 1 })
    expect(rows.filter(row => row.kind === 'tool')).toEqual([])
    expect(rows.every(row => !row.spanError)).toBe(true)
    expect(parent.spanData.events.map(event => event.name)).toEqual([
      'genuine-event',
    ])
    expect(evalSpanEvidence([parent.spanData], [log])[0]).toMatchObject({
      source: 'shell · Log event · OpenCode',
      toolError: true,
    })
    expect(
      evalSpanEvidence([parent.spanData], [log])[0].sourceDescription
    ).toContain('tool.time.completed')
    expect(evalEventTools([parent], [log, log])).toHaveLength(1)
    expect(
      evalEventTools([parent], [{ ...log, spanID: ref(3).spanID }])
    ).toEqual([])
    expect(
      evalEventTools([parent], [{ ...log, traceID: 'b'.repeat(32) }])
    ).toEqual([])
    expect(
      evalEventTools([parent], [log, { ...log, logRef: 'ambiguous' }])
    ).toEqual([])
    const native = node(3, 2, 2)
    native.spanData.attributes = [
      {
        key: 'gen_ai.operation.name',
        value: { kind: 'string', value: 'execute_tool' },
      },
      { key: 'gen_ai.tool.call.id', value: { kind: 'string', value: 'call' } },
    ]
    expect(evalEventTools([parent, native], [log])).toEqual([])
    expect(source.results[0].toolsKnown).toBe(false)
  })

  it('places received events and owned logs without duplicating logs on call-ID-linked spans', () => {
    const tool = node(2, 1, 1)
    tool.spanData.attributes = [
      {
        key: 'gen_ai.operation.name',
        value: { kind: 'string', value: 'execute_tool' },
      },
      {
        key: 'gen_ai.tool.call.id',
        value: { kind: 'string', value: 'one-call' },
      },
    ]
    tool.spanData.events = [
      {
        name: 'tool.output',
        timestamp: 12n,
        attributes: [],
        droppedAttributesCount: 0,
      },
    ]
    const records = evalTimelineRecords(tool, [
      {
        logRef: 'owned',
        spanID: ref(2).spanID,
        timestamp: 11n,
        severityNumber: 0,
        severityText: '',
        serviceName: '',
        eventName: 'owned',
        bodyPreview: '',
      },
      {
        logRef: 'not-owned',
        spanID: ref(1).spanID,
        timestamp: 11n,
        severityNumber: 0,
        severityText: '',
        serviceName: '',
        eventName: 'not-owned',
        bodyPreview: '',
      },
    ])
    expect(records.map(record => [record.kind, record.timestamp])).toEqual([
      ['log', 11n],
      ['event', 12n],
    ])
    expect(records[0]).toMatchObject({
      log: { logRef: 'owned', spanID: ref(2).spanID },
    })
  })

  it('counts rich Pi parent events without creating tool spans, timing or span errors', () => {
    const root = node(1, null, 0)
    const parent = node(2, 1, 1)
    parent.spanData.events = [
      {
        name: 'tool.execution',
        timestamp: parent.spanData.endTime,
        droppedAttributesCount: 0,
        attributes: [
          {
            key: 'pi.event.type',
            value: { kind: 'string', value: 'tool_execution_end' },
          },
          {
            key: 'gen_ai.tool.call.id',
            value: { kind: 'string', value: 'pi-call' },
          },
          { key: 'tool.is_error', value: { kind: 'bool', value: true } },
          {
            key: 'tool.output',
            value: { kind: 'string', value: 'exact\nerror' },
          },
        ],
      },
    ]
    const source = {
      ...exampleExperiments[0],
      telemetry: undefined,
      results: [
        {
          ...exampleExperiments[0].results[0],
          state: 'Complete' as const,
          telemetry: { traceID },
          resultTelemetry: undefined,
          tools: [],
          toolsKnown: false,
        },
      ],
    }
    const index = indexEvalTraces([
      { traceID, unplacedSpanCount: 0, spans: [root, parent] },
    ])
    const experiment = projectEvalTraceEvidence(source, index)
    expect(evalToolCounts(experiment.results[0])).toEqual({
      total: 1,
      failed: 1,
    })
    expect(experiment.results[0].tools).toEqual([])
    expect(experiment.results[0].assessments).toEqual(
      source.results[0].assessments
    )
    const rows = buildEvalOutline(
      groupResults(experiment.results, experiment.results),
      experiment,
      index
    )
    expect(rows.filter(row => row.kind === 'tool')).toEqual([])
    expect(rows.every(row => !row.spanError)).toBe(true)
    expect(
      rows.find(row => row.reference?.spanID === ref(2).spanID)?.toolCounts
    ).toEqual({ total: 1, failed: 1 })
    expect(evalSpanEvidence([parent.spanData])[0].fields).toEqual(
      parent.spanData.events[0].attributes
    )
    expect(source.results[0].toolsKnown).toBe(false)
    parent.spanData.events[0].attributes =
      parent.spanData.events[0].attributes.filter(
        attribute => attribute.key !== 'pi.event.type'
      )
    parent.spanData.events[0].attributes.push({
      key: 'opencode.runtime.version',
      value: { kind: 'string', value: '2.0.29' },
    })
    expect(evalEventTools([parent])).toHaveLength(1)

    const duplicate = node(3, 1, 1)
    duplicate.spanData.events = parent.spanData.events
    expect(evalEventTools([parent, duplicate])).toEqual([])
    duplicate.spanData.events = []
    duplicate.spanData.attributes = [
      {
        key: 'gen_ai.operation.name',
        value: { kind: 'string', value: 'execute_tool' },
      },
      {
        key: 'gen_ai.tool.call.id',
        value: { kind: 'string', value: 'pi-call' },
      },
    ]
    expect(evalEventTools([parent, duplicate])).toEqual([])
    parent.spanData.events[0].attributes.push({
      key: 'gen_ai.tool.call.id',
      value: { kind: 'string', value: 'conflict' },
    })
    expect(evalEventTools([parent])).toEqual([])
  })

  it('links unique recorded grader identities without rewriting judgments or guessing duplicate names', () => {
    const grader = node(2, 1, 1)
    grader.spanData.attributes = [
      {
        key: 'promptfoo.span.role',
        value: { kind: 'string', value: 'grader' },
      },
      {
        key: 'promptfoo.grader.id',
        value: { kind: 'string', value: 'llm-rubric' },
      },
      {
        key: 'gen_ai.evaluation.name',
        value: { kind: 'string', value: 'llm-rubric' },
      },
    ]
    const child = node(3, 2, 2)
    child.spanData.attributes = [
      {
        key: 'promptfoo.span.role',
        value: { kind: 'string', value: 'grader' },
      },
    ]
    const index = indexEvalTraces([
      {
        traceID,
        unplacedSpanCount: 0,
        spans: [node(1, null, 0), grader, child],
      },
    ])
    const assessment = {
      ...exampleExperiments[0].results[0].assessments[0]!,
      id: 'native-assessment',
      sourceName: 'llm-rubric',
      telemetry: undefined,
      value: 0.5,
      label: 'Partial',
    }
    const result = {
      ...exampleExperiments[0].results[0],
      repetition: 1,
      telemetry: { traceID },
      resultTelemetry: undefined,
      assessments: [assessment],
    }
    const source = { ...exampleExperiments[0], results: [result] }
    const experiment = projectEvalTraceEvidence(source, index)
    expect(experiment.results[0].assessments[0]).toEqual({
      ...assessment,
      telemetry: ref(2),
    })
    expect(source.results[0].assessments[0].telemetry).toBeUndefined()
    const rows = buildEvalOutline(
      groupResults(experiment.results, experiment.results),
      experiment,
      index
    )
    expect(
      rows.filter(row => row.kind === 'grader').map(row => row.assessmentID)
    ).toEqual(['native-assessment', 'native-assessment'])
    const ambiguous = {
      ...source,
      results: [
        {
          ...result,
          assessments: [assessment, { ...assessment, id: 'other' }],
        },
      ],
    }
    expect(
      projectEvalTraceEvidence(ambiguous, index).results[0].assessments.every(
        item => !item.telemetry
      )
    ).toBe(true)
    const duplicate = node(4, 1, 1)
    duplicate.spanData.attributes = grader.spanData.attributes
    const duplicatedIndex = indexEvalTraces([
      {
        traceID,
        unplacedSpanCount: 0,
        spans: [node(1, null, 0), grader, duplicate],
      },
    ])
    expect(
      projectEvalTraceEvidence(source, duplicatedIndex).results[0]
        .assessments[0].telemetry
    ).toBeUndefined()
  })

  it('keeps typed tool outputs and correlates ancestor logs only by a unique call ID', () => {
    const root = node(1, null, 0)
    const tool = node(2, 1, 1)
    tool.spanData.attributes = [
      {
        key: 'gen_ai.operation.name',
        value: { kind: 'string', value: 'execute_tool' },
      },
      {
        key: 'gen_ai.tool.call.id',
        value: { kind: 'string', value: 'call-a' },
      },
      {
        key: 'gen_ai.tool.call.result',
        value: { kind: 'int64', value: 9007199254740993n },
      },
    ]
    tool.spanData.events = [
      {
        name: 'tool.output',
        timestamp: tool.spanData.endTime,
        droppedAttributesCount: 0,
        attributes: [
          { key: 'output', value: { kind: 'string', value: '' } },
          {
            key: 'stderr',
            value: { kind: 'string', value: 'first\nsecond\nthird\nfourth' },
          },
        ],
      },
    ]
    const execution = node(3, 2, 2)
    const nestedTool = node(4, 2, 2)
    nestedTool.spanData.attributes = [
      {
        key: 'gen_ai.operation.name',
        value: { kind: 'string', value: 'execute_tool' },
      },
    ]
    const nodes = indexEvalTraces([
      {
        traceID,
        unplacedSpanCount: 0,
        spans: [root, tool, execution, nestedTool, node(5, 4, 3)],
      },
    ]).nodes
    const spans = toolEvidenceSpans(tool.spanData, nodes)
    expect(spans.map(span => span.spanID)).toEqual([
      ref(2).spanID,
      ref(3).spanID,
    ])
    expect(
      toolLogOwners(spans, nodes, uniqueToolCallID(tool.spanData, nodes)).map(
        span => span.spanID
      )
    ).toEqual([ref(2).spanID, ref(3).spanID, ref(1).spanID])
    const log: LogData = {
      logRef: 'log-a',
      traceID,
      spanID: ref(1).spanID,
      timestamp: 1n,
      observedTimestamp: 1n,
      severityText: '',
      severityNumber: 0,
      eventName: '',
      flags: 0,
      droppedAttributesCount: 0,
      resource: root.spanData.resource,
      scope: root.spanData.scope,
      body: { kind: 'string', value: 'claude_code.tool_result' },
      attributes: [
        { key: 'tool_use_id', value: { kind: 'string', value: 'call-a' } },
        {
          key: 'tool_input',
          value: { kind: 'string', value: '{"n":9007199254740993}' },
        },
      ],
    }
    const records = evalSpanEvidence(
      spans,
      [
        log,
        { ...log, logRef: 'other-trace', traceID: 'b'.repeat(32) },
        {
          ...log,
          logRef: 'other-call',
          attributes: [
            { key: 'tool_use_id', value: { kind: 'string', value: 'call-b' } },
          ],
        },
      ],
      'call-a'
    )
    expect(records.map(record => record.id)).toEqual([
      `${referenceKey(tool.spanData)}/attributes`,
      `${referenceKey(tool.spanData)}/event/0`,
      'log/log-a',
    ])
    expect(records[0].fields[1].value).toEqual({
      kind: 'int64',
      value: 9007199254740993n,
    })
    expect(records[1].fields[0].value).toEqual({ kind: 'string', value: '' })
    expect(records[2].fields.at(-1)?.value).toEqual({
      kind: 'string',
      value: '{"n":9007199254740993}',
    })
    expect(
      evalSpanEvidence(spans, [log]).some(record =>
        record.id.startsWith('log/')
      )
    ).toBe(false)
    nestedTool.spanData.attributes = tool.spanData.attributes
    expect(uniqueToolCallID(tool.spanData, nodes)).toBeUndefined()
  })

  it('resolves trace-only results and builds start-ordered native execution, tool and grader rows', () => {
    const root = node(1, null, 0)
    const target = node(2, 1, 1)
    target.spanData.attributes = [
      {
        key: 'promptfoo.span.role',
        value: { kind: 'string', value: 'target' },
      },
    ]
    const tool = node(3, 2, 2)
    tool.spanData.attributes = [
      {
        key: 'gen_ai.operation.name',
        value: { kind: 'string', value: 'execute_tool' },
      },
    ]
    const model = node(4, 2, 2)
    const grader = node(5, 1, 1)
    grader.spanData.attributes = [
      {
        key: 'promptfoo.span.role',
        value: { kind: 'string', value: 'grader' },
      },
    ]
    const index = indexEvalTraces([
      {
        traceID,
        unplacedSpanCount: 0,
        spans: [root, target, model, tool, grader],
      },
    ])
    const result = {
      ...exampleExperiments[0].results[0],
      repetition: 1,
      resultTelemetry: undefined,
      telemetry: { traceID },
      tools: [],
      assessments: [],
    }
    const source = {
      ...exampleExperiments[0],
      results: [result],
      telemetry: undefined,
    }
    expect(evalTraceReferences(source)).toEqual([{ traceID }])
    const experiment = projectEvalTraceEvidence(source, index)
    expect(experiment.results[0].resultTelemetry).toEqual(ref(1))
    expect(experiment.results[0].tools.map(tool => tool.telemetry)).toEqual([
      ref(3),
    ])
    expect(source.results[0].tools).toEqual([])
    const rows = buildEvalOutline(
      groupResults(experiment.results, experiment.results),
      experiment,
      index
    )
    expect(rows.map(row => row.kind)).toEqual([
      'experiment',
      'case',
      'result',
      'execution',
      'tool',
      'span',
      'grader',
    ])
    expect(rows.slice(2).map(row => row.reference)).toEqual([
      ref(1),
      ref(2),
      ref(3),
      ref(4),
      ref(5),
    ])
    expect(rows[2].toolCounts).toEqual({ total: 1, failed: 0 })
    expect(rows[3].toolCounts).toEqual({ total: 1, failed: 0 })
    expect(rows[6].toolCounts).toEqual({ total: 0, failed: 0 })
    expect(rows.slice(2).every(row => !row.tree.brokenParent)).toBe(true)
  })

  it('shows partial branches without borrowing a root and resolves late parents with stable span row IDs', () => {
    const root = node(1, null, 0)
    const parent = node(2, 1, 1)
    const child = node(3, 2, 2)
    const result = {
      ...exampleExperiments[0].results[0],
      repetition: 1,
      resultTelemetry: undefined,
      telemetry: { traceID },
      tools: [],
      assessments: [],
      seconds: undefined,
    }
    const source = {
      ...exampleExperiments[0],
      results: [result],
      telemetry: undefined,
    }
    const project = (spans: SpanNode[]) => {
      const index = indexEvalTraces([{ traceID, unplacedSpanCount: 0, spans }])
      const experiment = projectEvalTraceEvidence(source, index)
      return {
        index,
        rows: buildEvalOutline(
          groupResults(experiment.results, experiment.results),
          experiment,
          index
        ),
      }
    }
    const partial = project([{ ...child, depth: 0 }])
    expect(resolveEvalReference({ traceID }, partial.index)).toBeUndefined()
    expect(partial.rows[2].reference).toEqual({ traceID })
    expect(partial.rows[2].window).toEqual({
      startTime: child.spanData.startTime,
      endTime: child.spanData.endTime,
    })
    const before = partial.rows[3]
    expect(before.tree.brokenParent).toBe(true)
    const complete = project([root, parent, child])
    const after = complete.rows.find(row => row.id === before.id)!
    expect(after.tree.brokenParent).toBe(false)
    expect(after.parentID).not.toBe(before.parentID)
    expect(complete.rows[2].reference).toEqual(ref(1))
    expect(
      resolveEvalReference({ traceID }, project([root, node(4, null, 0)]).index)
    ).toBeUndefined()
  })

  it('keeps experiment totals and derived timing complete when a case is filtered out', () => {
    const first = {
      ...exampleExperiments[0].results[0],
      id: 'first',
      caseID: 'first-case',
      state: 'Complete' as const,
      resultTelemetry: ref(1),
      telemetry: undefined,
      tools: [],
      assessments: [],
      investigatorCost: 0,
      graderCost: 0,
    }
    const second = {
      ...first,
      id: 'second',
      caseID: 'second-case',
      resultTelemetry: ref(2),
      investigatorCost: 3,
      graderCost: 2,
    }
    const experiment = {
      ...exampleExperiments[0],
      telemetry: undefined,
      seconds: undefined,
      results: [first, second],
    }
    const early = node(1, null, 0)
    const late = node(2, null, 0, true)
    late.spanData.startTime += 100n
    late.spanData.endTime += 100n
    const rows = buildEvalOutline(
      groupResults([first], experiment.results),
      experiment,
      indexEvalTraces([{ traceID, unplacedSpanCount: 0, spans: [early, late] }])
    )
    const root = rows[0]
    expect(root.kind).toBe('experiment')
    expect(root.parentID).toBeNull()
    expect(root.estimate).toEqual({ total: 5, incomplete: false })
    expect(root.spanError).toBe(true)
    expect(root.window).toEqual({
      startTime: early.spanData.startTime,
      endTime: late.spanData.endTime,
    })
    expect(root.tree.childrenCount).toBe(1)
    expect(rows.find(row => row.kind === 'case')?.parentID).toBe(root.id)
    expect(visibleOutlineRows(rows, ['time'], { [root.id]: false })).toEqual([
      root,
    ])
  })

  it('prefers a recorded zero-length experiment span to wider child timings and never sums duration-only children', () => {
    const result = {
      ...exampleExperiments[0].results[0],
      telemetry: ref(2),
      resultTelemetry: undefined,
      tools: [],
      assessments: [],
      seconds: 4,
    }
    const experiment = {
      ...exampleExperiments[0],
      telemetry: ref(1),
      seconds: 0,
      results: [result],
    }
    const parent = node(1, null, 0)
    parent.spanData.endTime = parent.spanData.startTime
    const rows = buildEvalOutline(
      groupResults(experiment.results, experiment.results),
      experiment,
      indexEvalTraces([
        { traceID, unplacedSpanCount: 0, spans: [parent, node(2, 1, 1)] },
      ])
    )
    expect(rows[0].reference).toEqual(ref(1))
    expect(rows[0].window).toEqual({
      startTime: parent.spanData.startTime,
      endTime: parent.spanData.startTime,
    })
    const untraced = { ...experiment, telemetry: undefined, seconds: undefined }
    const fallback = buildEvalOutline(
      groupResults(untraced.results, untraced.results),
      untraced,
      indexEvalTraces([])
    )[0]
    expect(fallback.window).toBeUndefined()
    expect(fallback.seconds).toBeUndefined()
  })

  it('places the traced result above sibling execution and grading branches, with costs counted once', () => {
    const evidence = {
      ...exampleExperiments[0].results[0],
      state: 'Complete' as const,
      resultTelemetry: ref(1),
      telemetry: ref(2),
      investigatorCost: 10,
      graderCost: 2,
      tools: [
        {
          name: 'tool',
          state: 'OK' as const,
          message: '',
          telemetry: ref(3),
          cost: 1,
        },
      ],
      assessments: [
        {
          ...exampleExperiments[0].results[0].assessments[0]!,
          telemetry: ref(4),
          cost: 0.5,
        },
      ],
    }
    const experiment = { ...exampleExperiments[0], results: [evidence] }
    const index = indexEvalTraces([
      {
        traceID,
        unplacedSpanCount: 0,
        spans: [
          node(1, null, 0),
          node(2, 1, 1),
          node(3, 2, 2, true),
          node(4, 1, 1),
        ],
      },
    ])
    const rows = buildEvalOutline(
      groupResults(experiment.results, experiment.results),
      experiment,
      index
    )
    expect(rows.map(row => [row.kind, row.depth])).toEqual([
      ['experiment', 0],
      ['case', 1],
      ['result', 2],
      ['execution', 3],
      ['tool', 4],
      ['grader', 3],
    ])
    expect(rows[2].reference).toEqual(ref(1))
    expect(rows[3].reference).toEqual(ref(2))
    // Parent timing remains authoritative even if children extend past it.
    expect(rows[2].window).toEqual({
      startTime: node(1, null, 0).spanData.startTime,
      endTime: node(1, null, 0).spanData.endTime,
    })
    expect(rows[3].window).toEqual({
      startTime: node(2, 1, 1).spanData.startTime,
      endTime: node(2, 1, 1).spanData.endTime,
    })
    expect(rows[3].parentID).toBe(rows[5].parentID)
    expect(rows[4].tree.brokenParent).toBe(false)
    expect(rows.map(row => row.spanError)).toEqual([
      true,
      true,
      true,
      true,
      true,
      false,
    ])
    expect(rows.map(row => row.estimate.total)).toEqual([
      12, 12, 12, 10, 1, 0.5,
    ])
    expect(evalTraceReferences(experiment)).toContainEqual(ref(1))
    expect(visibleOutlineRows(rows, [], {}).map(row => row.kind)).toEqual([
      'experiment',
      'case',
    ])
    expect(
      visibleOutlineRows(rows, ['results'], {}).map(row => row.kind)
    ).toEqual(['experiment', 'case', 'result'])
    expect(visibleOutlineRows(rows, ['time'], {}).map(row => row.kind)).toEqual(
      ['experiment', 'case', 'result', 'execution', 'tool', 'grader']
    )
    expect(
      visibleOutlineRows(rows, ['time', 'cost'], {}).map(row => row.id)
    ).toEqual(visibleOutlineRows(rows, ['time'], {}).map(row => row.id))
    expect(
      visibleOutlineRows(rows, ['time'], { [rows[3].id]: false }).map(
        row => row.kind
      )
    ).toEqual(['experiment', 'case', 'result', 'execution', 'grader'])
    expect(
      visibleOutlineRows(rows, ['time'], { [rows[5].id]: true }).map(
        row => row.kind
      )
    ).toEqual(rows.map(row => row.kind))
    expect(visibleOutlineRows(rows, ['time'], { [rows[0].id]: false })).toEqual(
      [rows[0]]
    )
  })

  it.each(['missing', 'invalid'])(
    'uses phase-specific child extents when parent timing is %s',
    mode => {
      const result = {
        ...exampleExperiments[0].results[0],
        resultTelemetry: ref(1),
        telemetry: ref(2),
        tools: [3, 4].map(id => ({
          name: `tool ${id}`,
          state: 'OK' as const,
          message: '',
          telemetry: ref(id),
        })),
        assessments: [
          {
            ...exampleExperiments[0].results[0].assessments[0]!,
            telemetry: ref(5),
          },
        ],
      }
      const first = node(3, 2, 0)
      const last = node(4, 2, 0)
      last.spanData.startTime += 100n
      last.spanData.endTime += 100n
      const grader = node(5, 1, 0)
      grader.spanData.startTime += 200n
      grader.spanData.endTime += 200n
      const invalidParents = [node(1, null, 0), node(2, 1, 0)]
      for (const parent of invalidParents)
        parent.spanData.endTime = parent.spanData.startTime - 1n
      const experiment = { ...exampleExperiments[0], results: [result] }
      const rows = buildEvalOutline(
        groupResults(experiment.results, experiment.results),
        experiment,
        indexEvalTraces([
          {
            traceID,
            unplacedSpanCount: 0,
            spans: [
              ...(mode === 'invalid' ? invalidParents : []),
              first,
              last,
              grader,
            ],
          },
        ])
      )
      expect(rows.find(row => row.kind === 'execution')?.window).toEqual({
        startTime: first.spanData.startTime,
        endTime: last.spanData.endTime,
      })
      expect(rows.find(row => row.kind === 'grader')?.window).toEqual({
        startTime: grader.spanData.startTime,
        endTime: grader.spanData.endTime,
      })
      for (const kind of ['experiment', 'case', 'result'])
        expect(rows.find(row => row.kind === kind)?.window).toEqual({
          startTime: first.spanData.startTime,
          endTime: grader.spanData.endTime,
        })
    }
  )

  it('retains untraced tools and grading without inventing span timing or costs', () => {
    const result = {
      ...exampleExperiments[0].results[0],
      telemetry: undefined,
      seconds: undefined,
      investigatorCost: undefined,
      graderCost: undefined,
      tools: [{ name: 'untraced tool', state: 'OK' as const, message: '' }],
      assessments: [
        {
          ...exampleExperiments[0].results[0].assessments[0]!,
          telemetry: undefined,
        },
      ],
    }
    const experiment = { ...exampleExperiments[0], results: [result] }
    const rows = buildEvalOutline(
      groupResults(experiment.results, experiment.results),
      experiment,
      indexEvalTraces([])
    )
    expect(rows.map(row => row.kind)).toEqual([
      'experiment',
      'case',
      'result',
      'tool',
    ])
    expect(
      rows.every(
        row => !row.reference && !row.window && row.seconds === undefined
      )
    ).toBe(true)
    expect(rows.every(row => row.estimate.total === null)).toBe(true)
  })
  it('uses compressed ancestry, marks skipped parents, nests calls and propagates errors through collapsed branches', () => {
    const trace: TraceData = {
      traceID,
      unplacedSpanCount: 0,
      spans: [
        node(1, null, 0),
        node(3, 2, 1),
        node(4, 3, 2, true),
        node(5, 1, 1),
      ],
    }
    const evidence = {
      ...exampleExperiments[0].results[0],
      state: 'Complete' as const,
      telemetry: ref(1),
      assessments: [],
      tools: [5, 4, 3].map(id => ({
        name: `tool ${id}`,
        state: 'OK' as const,
        message: '',
        telemetry: ref(id),
      })),
    }
    const result = evalExecutionRows(evidence, indexEvalTraces([trace]))
    expect(result.rows.map(row => row.name)).toEqual([
      'tool 3',
      'tool 4',
      'tool 5',
    ])
    expect(result.rows.map(row => row.depth)).toEqual([1, 2, 1])
    expect(result.rows.map(row => row.tree.brokenParent)).toEqual([
      true,
      false,
      false,
    ])
    expect(result.rows.map(row => row.state)).toEqual([
      'Error',
      'Error',
      'Complete',
    ])
    expect(result.state).toBe('Error')
    expect(result.spanError).toBe(true)
    expect(result.internalFault).toBe(false)
    expect(
      evalExecutionRows(
        { ...evidence, state: 'Error' },
        indexEvalTraces([trace])
      ).internalFault
    ).toBe(true)
    expect(result.childrenCount).toBe(2)
    expect(
      visibleExecutionRows(result.rows, [result.rows[0]!.id]).map(
        row => row.name
      )
    ).toEqual(['tool 3', 'tool 5'])
    expect(evidence.state).toBe('Complete')
    expect(evidence.tools.every(tool => tool.state === 'OK')).toBe(true)
  })

  it('keeps missing evidence and grader references without inventing cross-trace ancestry', () => {
    const foreign = node(3, 1, 0)
    foreign.spanData.traceID = 'b'.repeat(32)
    const evidence = {
      ...exampleExperiments[0].results[0],
      telemetry: ref(1),
      tools: [
        {
          name: 'missing tool',
          state: 'OK' as const,
          message: '',
          seconds: 2,
          telemetry: ref(7),
        },
      ],
      assessments: [
        {
          ...exampleExperiments[0].results[0].assessments[0]!,
          telemetry: ref(3, foreign.spanData.traceID),
        },
      ],
    }
    const index = indexEvalTraces([
      { traceID, unplacedSpanCount: 0, spans: [node(1, null, 0)] },
      {
        traceID: foreign.spanData.traceID,
        unplacedSpanCount: 0,
        spans: [foreign],
      },
    ])
    const result = evalExecutionRows(evidence, index)
    expect(result.rows).toHaveLength(2)
    expect(
      result.rows.every(row => row.depth === 1 && row.tree.brokenParent)
    ).toBe(true)
    expect(result.rows.find(row => row.name === 'missing tool')).toMatchObject({
      seconds: 2,
      node: undefined,
    })
    expect(index.nodes.get(referenceKey(foreign.spanData))).toBe(foreign)
    expect(
      evalTraceReferences({ ...exampleExperiments[0], results: [evidence] })
    ).toContainEqual(evidence.assessments[0]!.telemetry)
  })
})
