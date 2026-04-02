import { EditorView } from '@codemirror/view';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { tags as t } from '@lezer/highlight';

// Theme-aware colors via --rp-* tokens defined in app.css
const qc = {
  base: 'var(--rp-base)',
  surface: 'var(--rp-surface)',
  overlay: 'var(--rp-overlay)',
  muted: 'var(--rp-muted)',
  subtle: 'var(--rp-subtle)',
  text: 'var(--rp-text)',
  iris: 'var(--rp-iris)',
  foam: 'var(--rp-foam)',
  gold: 'var(--rp-gold)',
  rose: 'var(--rp-rose)',
  love: 'var(--rp-love)',
};

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
]);

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
  // Completion popup
  '.cm-tooltip': {
    backgroundColor: qc.surface,
    border: `1px solid ${qc.overlay}`,
    borderRadius: '6px',
    boxShadow: '0 4px 16px rgba(0, 0, 0, 0.3)',
  },
  '.cm-tooltip-autocomplete': {
    '& > ul': {
      fontFamily: '"Atkinson Hyperlegible Mono", ui-monospace, monospace',
      fontSize: '13px',
    },
    '& > ul > li': {
      padding: '4px 8px',
    },
    '& > ul > li[aria-selected]': {
      backgroundColor: qc.overlay,
      color: qc.text,
    },
  },
  '.cm-completionLabel': {
    color: qc.text,
  },
  '.cm-completionDetail': {
    color: qc.muted,
    fontStyle: 'normal',
    marginLeft: '8px',
  },
  // Lint diagnostics
  '.cm-diagnostic-error': {
    borderLeft: `3px solid ${qc.love}`,
    backgroundColor: `${qc.love}10`,
    padding: '4px 8px',
    marginLeft: '4px',
  },
  '.cm-lintRange-error': {
    backgroundImage: 'none',
    textDecoration: `underline wavy ${qc.love}`,
    textUnderlineOffset: '3px',
  },
});

export const queryTheme = [
  queryEditorTheme,
  syntaxHighlighting(queryHighlightStyle),
];
