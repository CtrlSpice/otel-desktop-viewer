import type { ResourceData } from '@/types/api-types'

/** Extract `service.name` from a resource's attributes, if present. */
export function getServiceName(resource: ResourceData): string | undefined {
  return resource.attributes.find(a => a.key === 'service.name')?.value
}
