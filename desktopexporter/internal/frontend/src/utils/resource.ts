import type { ResourceData } from '@/types/api-types'

/** Extract `service.name` from a resource's attributes, if present. */
export function getServiceName(
  resource: ResourceData | { serviceName: string }
): string | undefined {
  if ('serviceName' in resource) return resource.serviceName
  const value = resource.attributes.find(a => a.key === 'service.name')?.value
  return value?.kind === 'string' ? value.value : undefined
}
