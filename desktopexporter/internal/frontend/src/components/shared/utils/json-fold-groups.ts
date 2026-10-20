import { RangeSet, StateEffect, StateField } from '@codemirror/state'
import {
  Decoration,
  EditorView,
  GutterMarker,
  ViewPlugin,
  gutterLineClass,
} from '@codemirror/view'
import {
  foldEffect,
  foldGutter,
  foldable,
  foldedRanges,
  syntaxTree,
  unfoldEffect,
} from '@codemirror/language'
import { createJsonFoldCaret } from './json-fold-caret'

type FoldHover = { from: number; line: number } | null
const setFoldHover = StateEffect.define<FoldHover>()
class ActiveFoldLine extends GutterMarker {
  elementClass = 'json-fold-group-active'
}
const activeLine = new ActiveFoldLine()
const foldHover = StateField.define<FoldHover>({
  create: () => null,
  update(value, transaction) {
    if (transaction.docChanged) return null
    for (const effect of transaction.effects)
      if (effect.is(setFoldHover)) value = effect.value
    return value
  },
  provide: field =>
    gutterLineClass.from(field, value =>
      value ? RangeSet.of(activeLine.range(value.line)) : RangeSet.empty
    ),
})

function hover(view: EditorView, from: number | null) {
  if ((view.state.field(foldHover)?.from ?? null) === from) return
  view.dispatch({
    effects: setFoldHover.of(
      from === null ? null : { from, line: view.state.doc.lineAt(from).from }
    ),
  })
}

function brackets(view: EditorView) {
  const marks: ReturnType<Decoration['range']>[] = []
  const seen = new Set<number>()
  const active = view.state.field(foldHover)?.from
  for (const { from, to } of view.visibleRanges) {
    syntaxTree(view.state).iterate({
      from,
      to,
      enter(node) {
        if (
          !['Object', 'Array'].includes(node.name) ||
          seen.has(node.from) ||
          node.to - node.from <= 2
        )
          return
        seen.add(node.from)
        // Incomplete raw previews have no closing partner and cannot be folded.
        const opening = view.state.doc.sliceString(node.from, node.from + 1)
        const closing = view.state.doc.sliceString(node.to - 1, node.to)
        if (!(
          (opening === '{' && closing === '}') ||
          (opening === '[' && closing === ']')
        ))
          return
        const decoration = Decoration.mark({
          class: `json-fold-bracket${active === node.from ? ' json-fold-bracket--active' : ''}`,
          attributes: {
            'data-json-fold': String(node.from),
            title: 'Toggle JSON section',
          },
        })
        marks.push(
          decoration.range(node.from, node.from + 1),
          decoration.range(node.to - 1, node.to)
        )
      },
    })
  }
  return Decoration.set(marks, true)
}

function toggle(view: EditorView, from: number) {
  let folded: { from: number; to: number } | undefined
  foldedRanges(view.state).between(from + 1, from + 1, (start, end) => {
    if (start === from + 1) folded = { from: start, to: end }
  })
  if (folded) {
    view.dispatch({ effects: unfoldEffect.of(folded) })
    return
  }
  let node = syntaxTree(view.state).resolveInner(from, 1)
  while (
    node.parent &&
    !(node.from === from && ['Object', 'Array'].includes(node.name))
  )
    node = node.parent
  if (node.from === from && ['Object', 'Array'].includes(node.name)) {
    view.dispatch({
      effects: foldEffect.of({ from: node.from + 1, to: node.to - 1 }),
    })
  }
}

/** Use CodeMirror's folding state for both the gutter and paired delimiters. */
export const jsonFoldGroups = [
  foldHover,
  ViewPlugin.define(
    view => ({
      decorations: brackets(view),
      pressed: null as { from: number; x: number; y: number } | null,
      update(update) {
        if (
          update.docChanged ||
          update.viewportChanged ||
          update.startState.field(foldHover) !==
            update.state.field(foldHover) ||
          syntaxTree(update.startState) !== syntaxTree(update.state) ||
          foldedRanges(update.startState) !== foldedRanges(update.state)
        ) {
          this.decorations = brackets(update.view)
        }
      },
    }),
    {
      decorations: plugin => plugin.decorations,
      eventHandlers: {
        mousedown(event) {
          const group =
            event.target instanceof Element
              ? event.target.closest<HTMLElement>('[data-json-fold]')
              : null
          this.pressed =
            event.button === 0 && event.detail === 1 && group
              ? {
                  from: Number(group.dataset.jsonFold),
                  x: event.clientX,
                  y: event.clientY,
                }
              : null
          return false
        },
        mouseup(event, view) {
          const pressed = this.pressed
          this.pressed = null
          if (
            !pressed ||
            event.button !== 0 ||
            event.shiftKey ||
            event.ctrlKey ||
            event.metaKey ||
            event.altKey ||
            Math.hypot(event.clientX - pressed.x, event.clientY - pressed.y) >
              4 ||
            !view.state.selection.main.empty ||
            !view.dom.ownerDocument.getSelection()?.isCollapsed
          )
            return false
          // Matching-bracket decorations may replace the original DOM target on press.
          toggle(view, pressed.from)
          return true
        },
      },
    }
  ),
  foldGutter({
    markerDOM(open) {
      const marker = document.createElement('span')
      marker.className = 'json-fold-marker'
      marker.title = open ? 'Fold line' : 'Unfold line'
      marker.dataset.open = String(open)
      marker.append(createJsonFoldCaret())
      return marker
    },
    domEventHandlers: {
      mousemove(view, line) {
        let from: number | null = null
        foldedRanges(view.state).between(line.from, line.to, start => {
          from = start - 1
        })
        const range = foldable(view.state, line.from, line.to)
        if (from === null && range) from = range.from - 1
        hover(view, from)
        return false
      },
      mouseout(view, _line, event) {
        const target = (event as MouseEvent).relatedTarget
        if (!(target instanceof Node) || !view.dom.contains(target))
          hover(view, null)
        return false
      },
    },
  }),
  EditorView.domEventHandlers({
    mousemove(event, view) {
      if (
        !(event.target instanceof Element) ||
        event.target.closest('.cm-gutters')
      )
        return false
      const group = event.target.closest<HTMLElement>('[data-json-fold]')
      hover(view, group ? Number(group.dataset.jsonFold) : null)
      return false
    },
    mouseleave(_event, view) {
      hover(view, null)
      return false
    },
  }),
  EditorView.theme({
    '.json-fold-bracket': { cursor: 'pointer' },
    '.json-fold-bracket--active, .json-fold-bracket--active *, .cm-foldGutter .json-fold-group-active':
      {
        color: 'var(--color-warning) !important',
      },
  }),
]
