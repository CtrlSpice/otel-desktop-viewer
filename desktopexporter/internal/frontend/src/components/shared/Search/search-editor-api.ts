import type { FieldDefinition } from '@/search/model'

/** Caller-owned fields and help for a workspace outside telemetry discovery. */
export type SearchDefinition = {
  path: string
  label: string
  fields: FieldDefinition[]
  examples: { label: string; query: string }[]
}

export type SearchEditorAPI = {
  submit: () => void
  clear: () => void
  /** Replace the visible query as one undoable edit and submit it. */
  setQuery?: (text: string) => void
}
