import {
  getOperatorsForFieldType,
  builtinFieldReference,
  type FieldDefinition,
  type FieldType,
} from '@/search/model'
import type { SearchDefinition } from '@/components/shared/Search/search-editor-api'

function field(
  name: string,
  type: FieldType,
  description: string
): FieldDefinition {
  return {
    name,
    type,
    description,
    ...builtinFieldReference(name, 'experiment'),
    operators: getOperatorsForFieldType(type),
  }
}

export const evalSearchDefinition: SearchDefinition = {
  path: '/evals',
  label: 'evals',
  fields: [
    field('name', 'string', 'Experiment name'),
    field('evaluation', 'string', 'Evaluation name'),
    field('source', 'string', 'Experiment source'),
    field('format', 'string', 'Source format'),
    field('id', 'string', 'Stored experiment ID'),
    field('datasetID', 'string', 'Stored dataset ID'),
    field('result.text', 'string', 'Received string values in one result'),
    field('result.ordinal', 'int64', 'Zero-based source record ordinal'),
    field('result.id', 'string', 'Received result ID'),
    field('result.provider', 'string', 'Received provider ID'),
    field('result.prompt', 'string', 'Received prompt ID'),
    field('result.output', 'string', 'Received string response output'),
    field('result.success', 'boolean', 'Received success flag'),
    field('result.score', 'float64', 'Received numeric score'),
  ],
  examples: [
    { label: 'Filter an experiment', query: 'name contains "checkout"' },
    {
      label: 'Match one result',
      query: 'result.provider = "model-a" and result.score < 0.5',
    },
    {
      label: 'Text and metadata',
      query: 'search("idempotency key") and name = "Experiment 2"',
    },
  ],
}
