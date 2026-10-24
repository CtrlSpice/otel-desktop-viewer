import { EditorView } from '@codemirror/view'

export function jsonEditorView(container: ParentNode): EditorView {
  const element = container.querySelector('.json-editor .cm-editor')
  if (!element) throw new Error('Expected a mounted JSON CodeMirror editor')
  const view = EditorView.findFromDOM(element as HTMLElement)
  if (!view) throw new Error('Expected the JSON editor view')
  return view
}
