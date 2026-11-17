import { describe, expect, it } from 'vitest'
import type { EventData, WaterfallEventData } from '@/types/api-types'
import {
  toolEventError,
  toolEventName,
  toolEventProvenance,
} from './tool-event'

describe('native waterfall event summaries', () => {
  it('preserves tool labels, error state and timestamp provenance without full event attributes', () => {
    const full: EventData = {
      name: 'tool.execution',
      timestamp: 9007199254740993n,
      droppedAttributesCount: 2,
      attributes: [
        { key: 'gen_ai.tool.name', value: { kind: 'string', value: 'shell' } },
        { key: 'tool.is_error', value: { kind: 'bool', value: true } },
        {
          key: 'opencode.event.timestamp.source',
          value: { kind: 'string', value: 'received timestamp' },
        },
        {
          key: 'large.unselected',
          get value(): never {
            throw new Error('must not decode unselected event payload')
          },
        },
      ],
    }
    const summary: WaterfallEventData = {
      name: full.name,
      timestamp: full.timestamp,
      droppedAttributesCount: full.droppedAttributesCount,
      summaryAttributes: full.attributes.slice(0, 3),
    }
    expect(toolEventName(summary)).toBe(toolEventName(full))
    expect(toolEventName(summary)).toBe('shell')
    expect(toolEventError(summary)).toBe(true)
    expect(toolEventProvenance(summary)).toEqual(toolEventProvenance(full))
    expect(toolEventProvenance(summary)?.description).toContain(
      'received timestamp'
    )
    expect(summary.timestamp).toBe(9007199254740993n)
    expect(summary).not.toHaveProperty('attributes')
  })
})
