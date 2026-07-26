import { describe, expect, it } from 'vitest'
import { heatmapSwatches, readableTextColor } from './chart-palette'

describe('heatmapSwatches', () => {
  it('returns the hot endpoint colour for a single step', () => {
    expect(heatmapSwatches(1, 'rose-pine')).toEqual(['#eb6f92'])
  })

  it('returns one colour per requested step', () => {
    expect(heatmapSwatches(4, 'rose-pine')).toHaveLength(4)
  })

  it('ends on the theme hot endpoint colour', () => {
    const swatches = heatmapSwatches(3, 'rose-pine-dawn')
    // The last sample position is t=1, so the interpolator lands exactly on
    // the hot endpoint -- just reformatted as `rgb(...)` rather than hex.
    expect(swatches.at(-1)).toBe('rgb(144, 122, 169)')
  })

  it('falls back to the moon ramp for an unknown theme', () => {
    expect(heatmapSwatches(1, 'not-a-theme')).toEqual(
      heatmapSwatches(1, 'rose-pine-moon')
    )
  })

  it('falls back to the moon ramp for an empty theme', () => {
    expect(heatmapSwatches(2)).toEqual(heatmapSwatches(2, 'rose-pine-moon'))
  })

  it('floors fractional steps and clamps non-positive steps to 1', () => {
    expect(heatmapSwatches(2.9, 'rose-pine')).toHaveLength(2)
    expect(heatmapSwatches(0, 'rose-pine')).toEqual(
      heatmapSwatches(1, 'rose-pine')
    )
    expect(heatmapSwatches(-5, 'rose-pine')).toEqual(
      heatmapSwatches(1, 'rose-pine')
    )
  })
})

describe('readableTextColor', () => {
  it('picks the on-light glyph for a bright 6-digit hex colour', () => {
    expect(readableTextColor('#ffffff')).toBe('var(--color-on-light)')
  })

  it('picks the on-dark glyph for a dark 6-digit hex colour', () => {
    expect(readableTextColor('#000000')).toBe('var(--color-on-dark)')
  })

  it('picks the on-dark glyph for a dark 3-digit hex colour', () => {
    expect(readableTextColor('#000')).toBe('var(--color-on-dark)')
  })

  it('picks the on-light glyph for a bright 3-digit hex colour', () => {
    expect(readableTextColor('#fff')).toBe('var(--color-on-light)')
  })

  it('parses an rgb(...) string', () => {
    expect(readableTextColor('rgb(255, 255, 255)')).toBe(
      'var(--color-on-light)'
    )
    expect(readableTextColor('rgb(0, 0, 0)')).toBe('var(--color-on-dark)')
  })

  it('falls back to the on-dark glyph for unparseable input', () => {
    expect(readableTextColor('not-a-colour')).toBe('var(--color-on-dark)')
    expect(readableTextColor('')).toBe('var(--color-on-dark)')
  })
})
