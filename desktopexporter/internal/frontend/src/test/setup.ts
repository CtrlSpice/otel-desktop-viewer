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

const storageIsBroken =
  typeof localStorage === 'undefined' ||
  typeof localStorage.getItem !== 'function'

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
