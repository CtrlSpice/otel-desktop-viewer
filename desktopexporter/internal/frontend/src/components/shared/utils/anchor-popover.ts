export type PopoverAnchor = 'inward' | 'outward' | 'below-end'

const POSITIONED_CLASS = 'anchor-popover--positioned'
const VIEWPORT_MARGIN_PX = 8

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

/** Fixed position relative to the trigger. */
export function positionAnchorPopover(
  trigger: HTMLElement,
  popover: HTMLElement,
  anchor: PopoverAnchor
): void {
  const rect = trigger.getBoundingClientRect()
  popover.style.position = 'fixed'
  popover.style.inset = 'auto'
  popover.style.bottom = 'auto'
  popover.style.transform = 'none'

  if (anchor === 'outward') {
    popover.style.top = `${rect.top}px`
    popover.style.left = `${rect.right + outwardGapPx()}px`
    popover.style.right = 'auto'
  } else if (anchor === 'below-end') {
    const gap = inwardGapPx()
    popover.style.top = `${rect.bottom + gap}px`
    popover.style.left = 'auto'
    popover.style.right = `${window.innerWidth - rect.right}px`

    const popRect = popover.getBoundingClientRect()
    if (popRect.width > 0 && popRect.left < VIEWPORT_MARGIN_PX) {
      popover.style.right = 'auto'
      popover.style.left = `${VIEWPORT_MARGIN_PX}px`
    }
  } else {
    popover.style.top = `${rect.bottom + inwardGapPx()}px`
    popover.style.left = `${rect.left}px`
    popover.style.right = 'auto'
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
  let positionRaf: number | null = null

  const applyPosition = () => {
    positionAnchorPopover(trigger, popover, anchor)
    popover.classList.add(POSITIONED_CLASS)
  }

  const schedulePosition = () => {
    if (positionRaf !== null) cancelAnimationFrame(positionRaf)
    positionRaf = requestAnimationFrame(() => {
      positionRaf = null
      applyPosition()
    })
  }

  const startReposition = () => {
    const reposition = (e: Event) => {
      const target = e.target
      if (target instanceof Node && popover.contains(target)) return
      schedulePosition()
    }
    const onResize = () => schedulePosition()
    window.addEventListener('resize', onResize)
    window.addEventListener('scroll', reposition, true)
    stopReposition = () => {
      window.removeEventListener('resize', onResize)
      window.removeEventListener('scroll', reposition, true)
    }
  }

  const handleBeforeToggle = (e: ToggleEvent) => {
    if (e.newState === 'open') {
      popover.classList.remove(POSITIONED_CLASS)
    }
  }

  const handleToggle = (e: ToggleEvent) => {
    const open = e.newState === 'open'
    onOpenChange(open)
    if (open) {
      schedulePosition()
      startReposition()
    } else {
      if (positionRaf !== null) {
        cancelAnimationFrame(positionRaf)
        positionRaf = null
      }
      popover.classList.remove(POSITIONED_CLASS)
      stopReposition?.()
      stopReposition = null
    }
  }

  popover.addEventListener('beforetoggle', handleBeforeToggle)
  popover.addEventListener('toggle', handleToggle)

  return () => {
    if (positionRaf !== null) cancelAnimationFrame(positionRaf)
    stopReposition?.()
    popover.removeEventListener('beforetoggle', handleBeforeToggle)
    popover.removeEventListener('toggle', handleToggle)
    popover.classList.remove(POSITIONED_CLASS)
  }
}
