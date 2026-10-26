# Shared styles

`app.css` is the stylesheet entry point and the Tailwind `@reference` target for
Svelte component styles. It imports:

- `tokens.css`: Tailwind theme registration, shared sizes, focus, motion and shell
  geometry. Seasonal colour values still come from `themes/tokens.ts`.
- `charts.css`: shared LayerChart chrome, legends, keyboard cursors and overlays.

`app.css` owns shared controls, tooltips/popovers, item states, tables, fields,
breakdown bars and resize handles. Component styles own their local layout and
geometry. Share an existing class when two components implement the same control;
keep unrelated layout rules local.

Subtle and Muted are registered theme colours, so `text-subtle` and `text-muted`
work in markup and `@apply` without custom utility aliases. UI and code fonts use
regular weight; the UI also loads the italic face for comments.

Preserve selector specificity and cascade order when moving a rule. LayerChart
and CodeMirror selectors can be used by their libraries even when no application
template contains the class name. Compact JSON syntax classes are generated from
token kinds. Check those consumers before removing apparently unused CSS.

Use the shared paper, field, focus and item-state tokens. Pane bodies retain the
uniform Kami surface; service-map cards retain Panel. Hover takes precedence over
selection, and focus is an independent outline. Reduced-motion rules belong with
the motion tokens.
