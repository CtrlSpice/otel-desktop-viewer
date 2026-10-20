import { EditorView } from '@codemirror/view'
import { syntaxHighlighting } from '@codemirror/language'
import { editorHighlightStyle } from '@/components/shared/utils/editor-highlighting'

// Tooltip styles injected as a global <style> because:
//   1. tooltips render in document.body (to escape backdrop-blur containment)
//   2. EditorView.theme() scopes under .cm-editor, which won't match body-parented tooltips
//   3. Tailwind tree-shakes unknown selectors from app.css
//
// Selectors use html[data-theme] to beat CM's baseTheme `&light`/`&dark` rules
// and follow the active Rosé Pine palette automatically.
let tooltipStylesInjected = false
function ensureTooltipStyles() {
  if (tooltipStylesInjected) return
  tooltipStylesInjected = true
  const style = document.createElement('style')
  style.textContent = `
    html[data-theme] .cm-tooltip {
      background-color: var(--color-base-100);
      border: 1px solid var(--color-base-300);
      border-radius: var(--radius-box);
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.25);
      color: var(--color-base-content);
      overflow: hidden;
    }
    html[data-theme] .cm-tooltip-autocomplete > ul {
      font-family: var(--font-sans);
      font-size: var(--text-base);
      line-height: var(--text-base--line-height);
      font-weight: 400;
      padding: 0;
    }
    html[data-theme] .cm-tooltip-autocomplete > ul > li {
      display: flex;
      align-items: center;
      height: auto;
      min-height: var(--table-row-h);
      padding: 0.25rem 1rem;
      gap: 0.5rem;
      box-sizing: border-box;
      border-radius: 0.75rem;
    }
    html[data-theme] .cm-tooltip-autocomplete > ul > li[aria-selected] {
      background-color: var(--item-selected-bg);
      color: var(--color-base-content);
    }
    html[data-theme] .cm-tooltip-autocomplete > ul > li:hover {
      background-color: var(--item-hover-bg);
    }
    html[data-theme] .cm-completionLabel {
      color: var(--color-base-content);
      padding-left: 0.25rem;
    }
    html[data-theme] .cm-completionDetail {
      color: var(--color-secondary);
      font-style: normal;
      font-size: var(--text-base);
      margin: 0;
    }
    html[data-theme] .cm-completionDetail::before {
      content: '· ';
      color: var(--color-subtle);
    }
  `
  document.head.appendChild(style)
}

import { editorColors as qc } from '@/components/shared/utils/editor-colors'

export const queryEditorTheme = EditorView.theme({
  '&': {
    fontSize: 'var(--text-base)',
    fontFamily: '"Atkinson Hyperlegible Mono", ui-monospace, monospace',
    backgroundColor: 'transparent',
    color: qc.text,
  },
  '.cm-content': {
    caretColor: qc.iris,
    padding: '0',
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: qc.iris,
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-selectionBackground': {
    backgroundColor: `${qc.highlightMed} !important`,
  },
  '.cm-activeLine': {
    backgroundColor: 'transparent',
  },
  '.cm-placeholder': {
    color: qc.subtle,
    fontSize: 'var(--text-base)',
    fontFamily: '"Atkinson Hyperlegible Mono", ui-monospace, monospace',
    fontStyle: 'normal',
    lineHeight: 'var(--text-base--line-height)',
  },
  // Lint diagnostics (render inline in .cm-editor, so EditorView.theme works)
  '.cm-diagnostic-error': {
    borderLeft: `3px solid ${qc.love}`,
    backgroundColor: 'color-mix(in oklab, var(--color-error) 12%, transparent)',
    padding: '4px 8px',
    marginLeft: '4px',
  },
  '.cm-lintRange-error': {
    backgroundImage: 'none',
    textDecoration: `underline wavy ${qc.love}`,
    textUnderlineOffset: '3px',
  },
})

export { ensureTooltipStyles }

export const queryTheme = [
  queryEditorTheme,
  syntaxHighlighting(editorHighlightStyle),
]
