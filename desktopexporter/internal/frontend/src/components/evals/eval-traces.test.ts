import { describe, expect, it } from 'vitest'
import type { SpanNode, TraceData } from '@/types/api-types'
import { exampleRuns } from './examples'
import {
  evalExecutionRows,
  evalTraceReferences,
  indexEvalTraces,
  referenceKey,
  visibleExecutionRows,
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
    const attempt = {
      ...exampleRuns[0].attempts[0],
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
    const result = evalExecutionRows(attempt, indexEvalTraces([trace]))
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
        { ...attempt, state: 'Error' },
        indexEvalTraces([trace])
      ).internalFault
    ).toBe(true)
    expect(result.childrenCount).toBe(2)
    expect(
      visibleExecutionRows(result.rows, [result.rows[0]!.id]).map(
        row => row.name
      )
    ).toEqual(['tool 3', 'tool 5'])
    expect(attempt.state).toBe('Complete')
    expect(attempt.tools.every(tool => tool.state === 'OK')).toBe(true)
  })

  it('keeps missing evidence and grader references without inventing cross-trace ancestry', () => {
    const foreign = node(3, 1, 0)
    foreign.spanData.traceID = 'b'.repeat(32)
    const attempt = {
      ...exampleRuns[0].attempts[0],
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
          ...exampleRuns[0].attempts[0].assessments[0]!,
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
    const result = evalExecutionRows(attempt, index)
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
      evalTraceReferences({ ...exampleRuns[0], attempts: [attempt] })
    ).toContainEqual(attempt.assessments[0]!.telemetry)
  })
})
