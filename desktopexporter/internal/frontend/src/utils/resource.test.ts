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

  it('rejects a non-string service name that bypassed wire validation', () => {
    const resource = resourceWith([
      { key: 'service.name', value: 'checkout', type: 'string' },
    ])
    Object.defineProperty(resource.attributes[0], 'value', { value: 123 })
    expect(getServiceName(resource)).toBeUndefined()
  })
})
