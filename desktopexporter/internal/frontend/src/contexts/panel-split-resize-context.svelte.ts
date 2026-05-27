/** Lets a stacked panel's header act as the vertical split resize handle. */
export const PANEL_SPLIT_RESIZE_KEY = Symbol('panelSplitResize')

export type PanelSplitResizeContext = {
  registerHandle: (el: HTMLElement | null) => void
  onPointerDown: (e: PointerEvent) => void
  onPointerMove: (e: PointerEvent) => void
  onPointerUp: (e: PointerEvent) => void
  onDoubleClick: () => void
  onKeydown: (e: KeyboardEvent) => void
  readonly isDragging: boolean
  readonly ariaNow: number
  readonly ariaMin: number
  readonly ariaMax: number
}
