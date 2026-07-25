import { describe, expect, it } from 'vitest'
import type { Attribute, ResourceData } from '@/types/api-types'
import { getServiceName } from './resource'

function resourceWith(attributes: Attribute[]): ResourceData {
  return { attributes, droppedAttributesCount: 0 }
}

describe('getServiceName', () => {
  it('returns the value when service.name is present', () => {
    const resource = resourceWith([
      { key: 'service.name', value: 'checkout', type: 'string' },
    ])
    expect(getServiceName(resource)).toBe('checkout')
  })

  it('returns undefined when service.name is absent', () => {
    const resource = resourceWith([
      { key: 'host.name', value: 'my-host', type: 'string' },
    ])
    expect(getServiceName(resource)).toBeUndefined()
  })

  it('returns undefined when there are no attributes', () => {
    expect(getServiceName(resourceWith([]))).toBeUndefined()
  })

  it('returns whatever value is stored, even if not a string at runtime', () => {
    // The Attribute type declares `value: string`, but the function performs
    // no runtime check -- it returns whatever is found. This models data that
    // has bypassed the type system (e.g. parsed straight from JSON).
    const resource = {
      attributes: [{ key: 'service.name', value: 123, type: 'int' }],
      droppedAttributesCount: 0,
    } as unknown as ResourceData
    expect(getServiceName(resource)).toBe(123)
  })
})
