import { type Diagnostic, linter } from '@codemirror/lint'
import type { EditorView } from '@codemirror/view'
import { validateQuery, type ValidationError } from '../queryParser'
import type { FieldDefinition } from '@/constants/fields'

export function createQueryLinter(getFields: () => FieldDefinition[]) {
  return linter((view: EditorView): Diagnostic[] => {
    const text = view.state.doc.toString()
    if (!text.trim()) return []

    const errors: ValidationError[] = validateQuery(text, getFields())

    return errors.map(err => ({
      from: Math.min(err.from, text.length),
      to: Math.min(err.to, text.length),
      severity: 'error' as const,
      message: err.message,
    }))
  })
}
