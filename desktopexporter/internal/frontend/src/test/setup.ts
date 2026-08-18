import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'

// Node >= 22 ships an experimental global `localStorage` that is present but
// non-functional unless the process is started with --localstorage-file. It
// shadows jsdom's working implementation in DOM test environments, so replace
// it with a real in-memory Storage whenever the global one is broken.
function createMemoryStorage(): Storage {
  const store = new Map<string, string>()
  return {
    get length() {
      return store.size
    },
    clear: () => {
      store.clear()
    },
    getItem: (key: string) => store.get(key) ?? null,
    key: (index: number) => [...store.keys()][index] ?? null,
    removeItem: (key: string) => {
      store.delete(key)
    },
    setItem: (key: string, value: string) => {
      store.set(String(key), String(value))
    },
  }
}

// Node's experimental global implements some of Storage and not the rest, so
// probing a single method is not enough -- it has getItem but no clear, which
// only surfaced when a test tried to reset between cases.
const storageIsBroken =
  typeof localStorage === 'undefined' ||
  typeof localStorage.getItem !== 'function' ||
  typeof localStorage.clear !== 'function' ||
  typeof localStorage.key !== 'function'

if (typeof document !== 'undefined' && storageIsBroken) {
  Object.defineProperty(globalThis, 'localStorage', {
    value: createMemoryStorage(),
    configurable: true,
    writable: true,
  })
}

// jsdom implements neither matchMedia (ThemeToggle's dark-scheme check) nor
// ResizeObserver (bits-ui, charts); both get inert stand-ins.
if (typeof window !== 'undefined' && typeof window.matchMedia !== 'function') {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    writable: true,
    value: (query: string): MediaQueryList =>
      ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      }) as MediaQueryList,
  })
}

if (
  typeof window !== 'undefined' &&
  typeof globalThis.ResizeObserver === 'undefined'
) {
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  globalThis.ResizeObserver =
    ResizeObserverStub as unknown as typeof ResizeObserver
}

// jsdom lacks the Web Animations API that Svelte transitions/animations use.
// This stub completes instantly (onfinish fires on the next microtask) so
// transitions resolve rather than hang or throw.
if (
  typeof Element !== 'undefined' &&
  typeof Element.prototype.animate !== 'function'
) {
  Element.prototype.animate = function (): Animation {
    const animation = {
      onfinish: null as ((event?: unknown) => void) | null,
      oncancel: null,
      cancel() {},
      finish() {},
      pause() {},
      play() {},
      reverse() {},
      finished: Promise.resolve(),
      playState: 'finished',
      currentTime: 0,
      startTime: 0,
      effect: null,
    }
    queueMicrotask(() => animation.onfinish?.())
    return animation as unknown as Animation
  }
}

// jsdom ships the popover UA stylesheet but not the popover JS API. This
// polyfill keeps real semantics: the `popover` attribute is never touched (it
// declares popover-ness, it is not open state). Open state lives in a WeakSet,
// visibility is forced with inline display (jsdom's UA sheet hides [popover]
// and can never match :popover-open), and beforetoggle/toggle events carry
// spec-shaped newState/oldState.
if (
  typeof HTMLElement !== 'undefined' &&
  typeof HTMLElement.prototype.showPopover !== 'function'
) {
  const openPopovers = new WeakSet<HTMLElement>()

  class ToggleEventPolyfill extends Event {
    readonly newState: string
    readonly oldState: string
    constructor(type: string, init: { newState: string; oldState: string }) {
      super(type)
      this.newState = init.newState
      this.oldState = init.oldState
    }
  }

  const setPopoverOpen = (el: HTMLElement, open: boolean): void => {
    const oldState = openPopovers.has(el) ? 'open' : 'closed'
    const newState = open ? 'open' : 'closed'
    if (oldState === newState) return
    el.dispatchEvent(
      new ToggleEventPolyfill('beforetoggle', { newState, oldState })
    )
    if (open) {
      openPopovers.add(el)
      el.style.display = 'block'
    } else {
      openPopovers.delete(el)
      el.style.removeProperty('display')
    }
    el.dispatchEvent(new ToggleEventPolyfill('toggle', { newState, oldState }))
  }

  HTMLElement.prototype.showPopover = function () {
    setPopoverOpen(this, true)
  }
  HTMLElement.prototype.hidePopover = function () {
    setPopoverOpen(this, false)
  }
  HTMLElement.prototype.togglePopover = function (
    this: HTMLElement,
    force?: boolean
  ) {
    const open = force ?? !openPopovers.has(this)
    setPopoverOpen(this, open)
    return open
  }

  // Declarative invokers (button popovertarget=...) are part of the same
  // missing API. Default action is toggle, per spec. Light dismiss is NOT
  // implemented; close popovers explicitly in tests when it matters.
  document.addEventListener('click', event => {
    const target = event.target
    if (!(target instanceof Element)) return
    const invoker = target.closest('button[popovertarget]')
    if (!invoker || (invoker as HTMLButtonElement).disabled) return
    const id = invoker.getAttribute('popovertarget')
    const popover = id ? document.getElementById(id) : null
    if (!(popover instanceof HTMLElement) || !popover.hasAttribute('popover')) {
      return
    }
    const action = invoker.getAttribute('popovertargetaction') ?? 'toggle'
    if (action === 'show') popover.showPopover()
    else if (action === 'hide') popover.hidePopover()
    else popover.togglePopover()
  })
}

// time-context persists selections to localStorage; clear between tests so
// component tests stay order-independent.
afterEach(() => {
  if (
    typeof localStorage !== 'undefined' &&
    typeof localStorage.clear === 'function'
  ) {
    localStorage.clear()
  }
})
