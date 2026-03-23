import { EditorView } from '@codemirror/view';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { tags as t } from '@lezer/highlight';

// Rosé Pine Moon palette
const rp = {
  base: '#232136',
  surface: '#2a273f',
  overlay: '#393552',
  muted: '#6e6a86',
  subtle: '#908caa',
  text: '#e0def4',
  iris: '#c4a7e7',
  foam: '#9ccfd8',
  pine: '#3e8fb0',
  gold: '#f6c177',
  rose: '#ea9ab2',
  love: '#eb6f92',
};

const queryHighlightStyle = HighlightStyle.define([
  { tag: t.propertyName, color: rp.foam },
  { tag: t.compareOperator, color: rp.subtle },
  { tag: t.operatorKeyword, color: rp.subtle },
  { tag: t.logicOperator, color: rp.iris, fontWeight: '600' },
  { tag: t.string, color: rp.gold },
  { tag: t.literal, color: rp.rose },
  { tag: t.null, color: rp.muted, fontStyle: 'italic' },
  { tag: t.paren, color: rp.subtle },
  { tag: t.squareBracket, color: rp.subtle },
]);

export const queryEditorTheme = EditorView.theme({
  '&': {
    fontSize: '14px',
    fontFamily: '"Atkinson Hyperlegible Mono", ui-monospace, monospace',
    backgroundColor: 'transparent',
    color: rp.text,
  },
  '.cm-content': {
    caretColor: rp.iris,
    padding: '8px 0',
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: rp.iris,
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-selectionBackground': {
    backgroundColor: `${rp.overlay} !important`,
  },
  '.cm-activeLine': {
    backgroundColor: 'transparent',
  },
  '.cm-placeholder': {
    color: rp.muted,
  },
  // Completion popup
  '.cm-tooltip': {
    backgroundColor: rp.surface,
    border: `1px solid ${rp.overlay}`,
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
      backgroundColor: rp.overlay,
      color: rp.text,
    },
  },
  '.cm-completionLabel': {
    color: rp.text,
  },
  '.cm-completionDetail': {
    color: rp.muted,
    fontStyle: 'normal',
    marginLeft: '8px',
  },
  // Lint diagnostics
  '.cm-diagnostic-error': {
    borderLeft: `3px solid ${rp.love}`,
    backgroundColor: `${rp.love}10`,
    padding: '4px 8px',
    marginLeft: '4px',
  },
  '.cm-lintRange-error': {
    backgroundImage: 'none',
    textDecoration: `underline wavy ${rp.love}`,
    textUnderlineOffset: '3px',
  },
});

export const queryTheme = [
  queryEditorTheme,
  syntaxHighlighting(queryHighlightStyle),
];
