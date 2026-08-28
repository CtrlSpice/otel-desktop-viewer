// @vitest-environment jsdom
import { describe, expect, it, afterEach } from 'vitest'

import {
  PANEL_DEFAULT_REM,
  PANEL_MIN_REM,
  PANEL_MAX_REM,
  remToPx,
  clampPanelRem,
  remToFraction,
} from './panel-width'

afterEach(() => {
  document.documentElement.style.fontSize = ''
})

describe('panel width rules', () => {
  it('keeps the default inside the bounds', () => {
    expect(PANEL_DEFAULT_REM).toBeGreaterThan(PANEL_MIN_REM)
    expect(PANEL_DEFAULT_REM).toBeLessThan(PANEL_MAX_REM)
  })

  it('converts rem against the live root font size, not an assumed 16', () => {
    document.documentElement.style.fontSize = '20px'
    expect(remToPx(10)).toBe(200)
  })

  it('falls back to 16 when the root font size is unreadable', () => {
    // jsdom reports '' for an unset font-size; a browser never does, but the
    // fallback keeps the math finite either way.
    expect(remToPx(PANEL_DEFAULT_REM)).toBe(PANEL_DEFAULT_REM * 16)
  })

  it('clamps at both bounds', () => {
    expect(clampPanelRem(PANEL_MIN_REM - 5)).toBe(PANEL_MIN_REM)
    expect(clampPanelRem(PANEL_MAX_REM + 5)).toBe(PANEL_MAX_REM)
    expect(clampPanelRem(30)).toBe(30)
  })

  it('turns NaN into the default rather than propagating it', () => {
    expect(clampPanelRem(Number.NaN)).toBe(PANEL_DEFAULT_REM)
    expect(clampPanelRem(Number.POSITIVE_INFINITY)).toBe(PANEL_DEFAULT_REM)
  })

  it('expresses a rem width as a fraction of a measured container', () => {
    // 28rem at 16px root = 448px; of a 1000px container that is 0.448.
    expect(remToFraction(PANEL_DEFAULT_REM, 1000)).toBeCloseTo(0.448)
  })

  it('yields zero for an unmeasured container instead of dividing by it', () => {
    expect(remToFraction(PANEL_DEFAULT_REM, 0)).toBe(0)
  })
})
