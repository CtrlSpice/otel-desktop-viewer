export type PopoverAnchor = 'inward' | 'outward'

function rootFontSizePx(): number {
  return parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
}

function inwardGapPx(): number {
  return rootFontSizePx() * 0.5
}

function outwardGapPx(): number {
  return rootFontSizePx() * 1.5
}

export function createPopoverId(prefix: string): string {
  return `${prefix}-${Math.random().toString(36).slice(2, 8)}`
}

/** Fixed position below (inward) or to the right (outward) of the trigger. */
export function positionAnchorPopover(
  trigger: HTMLElement,
  popover: HTMLElement,
  anchor: PopoverAnchor
): void {
  const rect = trigger.getBoundingClientRect()
  popover.style.position = 'fixed'
  popover.style.inset = 'auto'
  popover.style.right = 'auto'
  popover.style.bottom = 'auto'

  if (anchor === 'outward') {
    popover.style.top = `${rect.top}px`
    popover.style.left = `${rect.right + outwardGapPx()}px`
  } else {
    popover.style.top = `${rect.bottom + inwardGapPx()}px`
    popover.style.left = `${rect.left}px`
  }
}

/** Toggle listeners + resize/scroll reposition while open. Returns cleanup. */
export function setupAnchorPopover(options: {
  popover: HTMLDivElement
  trigger: HTMLElement
  anchor: PopoverAnchor
  onOpenChange: (open: boolean) => void
}): () => void {
  const { popover, trigger, anchor, onOpenChange } = options

  let stopReposition: (() => void) | null = null

  const position = () => positionAnchorPopover(trigger, popover, anchor)

  const startReposition = () => {
    const reposition = () => position()
    window.addEventListener('resize', reposition)
    window.addEventListener('scroll', reposition, true)
    stopReposition = () => {
      window.removeEventListener('resize', reposition)
      window.removeEventListener('scroll', reposition, true)
    }
  }

  const handleBeforeToggle = (e: ToggleEvent) => {
    if (e.newState === 'open') position()
  }

  const handleToggle = (e: ToggleEvent) => {
    const open = e.newState === 'open'
    onOpenChange(open)
    if (open) {
      position()
      startReposition()
    } else {
      stopReposition?.()
      stopReposition = null
    }
  }

  popover.addEventListener('beforetoggle', handleBeforeToggle)
  popover.addEventListener('toggle', handleToggle)

  return () => {
    stopReposition?.()
    popover.removeEventListener('beforetoggle', handleBeforeToggle)
    popover.removeEventListener('toggle', handleToggle)
  }
}
