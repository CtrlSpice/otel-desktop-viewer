import { EditorView } from '@codemirror/view'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags as t } from '@lezer/highlight'

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
      font-family: "Atkinson Hyperlegible Mono", ui-monospace, monospace;
      font-size: 13px;
      padding: 0;
    }
    html[data-theme] .cm-tooltip-autocomplete > ul > li {
      display: flex;
      align-items: center;
      height: var(--table-row-h);
      min-height: var(--table-row-h);
      padding: 0.25rem 1rem;
      gap: 0.5rem;
      box-sizing: border-box;
    }
    html[data-theme] .cm-tooltip-autocomplete > ul > li:nth-child(odd) {
      background-color: var(--table-zebra-bg);
    }
    html[data-theme] .cm-tooltip-autocomplete > ul > li:hover {
      background-color: var(--table-hover-bg);
    }
    html[data-theme] .cm-tooltip-autocomplete > ul > li[aria-selected] {
      background-color: var(--color-base-300);
      color: var(--color-base-content);
    }
    html[data-theme] .cm-completionLabel {
      color: var(--color-base-content);
      padding-left: 0.25rem;
    }
    html[data-theme] .cm-completionDetail {
      --badge-color: var(--color-secondary);
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: fit-content;
      height: calc(var(--size-selector, 0.25rem) * 5);
      padding-inline: calc(calc(var(--size-selector, 0.25rem) * 5) / 2 - var(--border, 1px));
      border-radius: var(--radius-selector, 0.25rem);
      border: var(--border, 1px) solid color-mix(in oklab, var(--badge-color) 10%, var(--color-base-100));
      background-color: color-mix(in oklab, var(--badge-color) 8%, var(--color-base-100));
      color: var(--badge-color);
      font-style: normal;
      font-size: 0.75rem;
      margin-left: auto;
      margin-right: 0.25rem;
      vertical-align: middle;
    }
  `
  document.head.appendChild(style)
}

// Theme-aware colors: DaisyUI --color-* plus --color-rose / --color-subtle (app.css)
const qc = {
  base: 'var(--color-base-100)',
  surface: 'var(--color-base-200)',
  overlay: 'var(--color-base-300)',
  muted: 'var(--color-neutral)',
  subtle: 'var(--color-subtle)',
  text: 'var(--color-base-content)',
  iris: 'var(--color-primary)',
  foam: 'var(--color-accent)',
  gold: 'var(--color-warning)',
  rose: 'var(--color-rose)',
  love: 'var(--color-error)',
}

const queryHighlightStyle = HighlightStyle.define([
  { tag: t.propertyName, color: qc.foam },
  { tag: t.compareOperator, color: qc.subtle },
  { tag: t.operatorKeyword, color: qc.subtle },
  { tag: t.logicOperator, color: qc.iris, fontWeight: '600' },
  { tag: t.string, color: qc.gold },
  { tag: t.literal, color: qc.rose },
  { tag: t.null, color: qc.muted, fontStyle: 'italic' },
  { tag: t.paren, color: qc.subtle },
  { tag: t.squareBracket, color: qc.subtle },
])

export const queryEditorTheme = EditorView.theme({
  '&': {
    fontSize: '14px',
    fontFamily: '"Atkinson Hyperlegible Mono", ui-monospace, monospace',
    backgroundColor: 'transparent',
    color: qc.text,
  },
  '.cm-content': {
    caretColor: qc.iris,
    padding: '8px 0',
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: qc.iris,
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-selectionBackground': {
    backgroundColor: `${qc.overlay} !important`,
  },
  '.cm-activeLine': {
    backgroundColor: 'transparent',
  },
  '.cm-placeholder': {
    color: qc.muted,
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
  syntaxHighlighting(queryHighlightStyle),
]
