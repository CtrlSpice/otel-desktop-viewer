import { describe, expect, it } from 'vitest'
import { resizeSegments } from './resize-segments'

describe('resize regions', () => {
  const rows = [
    { top: 40, bottom: 80 },
    { top: 160, bottom: 200 },
  ]
  it('leaves spanning rows out of the hit strip', () => {
    expect(resizeSegments(240, 40, 0, rows)).toEqual([
      { top: 0, bottom: 40 },
      { top: 80, bottom: 160 },
      { top: 200, bottom: 240 },
    ])
  })
  it('keeps the sticky header usable when a spanning row scrolls behind it', () => {
    expect(resizeSegments(180, 40, 60, rows)).toEqual([
      { top: 0, bottom: 40 },
      { top: 40, bottom: 100 },
      { top: 140, bottom: 180 },
    ])
  })
  it('clips exclusion and resize regions to the used table height', () => {
    expect(resizeSegments(170, 40, 0, rows)).toEqual([
      { top: 0, bottom: 40 },
      { top: 80, bottom: 160 },
    ])
  })
})
