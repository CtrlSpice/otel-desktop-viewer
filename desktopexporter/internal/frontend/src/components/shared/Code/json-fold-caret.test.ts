// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { ChevronRightIcon } from '@hugeicons/core-free-icons'
import { createJsonFoldCaret, JSON_FOLD_CARET } from './json-fold-caret'

describe('shared JSON fold caret', () => {
  it('uses the actual Hugeicons chevron data in the raw gutter, not a CSS triangle', () => {
    expect(JSON_FOLD_CARET).toBe(ChevronRightIcon)
    const icon = createJsonFoldCaret()
    expect(icon).toHaveAttribute('viewBox', '0 0 24 24')
    expect(icon).toHaveAttribute('aria-hidden', 'true')
    expect(icon.querySelector('path')).toHaveAttribute(
      'stroke-linecap',
      'round'
    )
    expect(icon.querySelector('path')).toHaveAttribute('stroke-width', '1.5')
    expect(icon).toHaveClass('json-fold-icon')
  })
})
