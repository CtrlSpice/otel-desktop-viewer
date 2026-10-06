import type { FieldDefinition, SearchSignal } from '@/search/model'
import { telemetryAPI } from './telemetry-service'

export async function getDynamicAttributes(
  signal: SearchSignal
): Promise<FieldDefinition[]> {
  switch (signal) {
    case 'traces':
      try {
        const attributes = await telemetryAPI.getTraceAttributeDefinitions()
        return attributes
      } catch (error) {
        console.warn('Failed to load dynamic attributes:', error)
        return []
      }

    case 'logs':
      try {
        const attributes = await telemetryAPI.getLogAttributeDefinitions()
        return attributes
      } catch (error) {
        console.warn('Failed to load dynamic log attributes:', error)
        return []
      }

    case 'metrics':
      try {
        const attributes = await telemetryAPI.getMetricAttributeDefinitions()
        return attributes
      } catch (error) {
        console.warn('Failed to load dynamic metric attributes:', error)
        return []
      }
    default:
      console.warn('Unknown signal type: ', signal)
      return []
  }
}
