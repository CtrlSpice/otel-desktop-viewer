// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import ErrorNavigationBadge from './ErrorNavigationBadge.svelte'

describe('ErrorNavigationBadge position', () => {
  it('separates the position from the err label while retaining the live announcement', () => {
    const { container } = render(ErrorNavigationBadge, {
      count: 2,
      index: 1,
      previous: false,
      next: true,
      onPrevious: vi.fn(),
      onNext: vi.fn(),
    })
    const position = screen.getByText('(1/2)')
    expect(position).toHaveClass('error-nav__position')
    expect(position.parentElement).toHaveTextContent('err (1/2)')
    expect(position.parentElement).toHaveAttribute('aria-live', 'polite')
    expect(container.querySelector('.error-nav__dot')).toHaveAttribute('aria-hidden', 'true')
  })

  it('omits the position when no error is selected', () => {
    const { container } = render(ErrorNavigationBadge, {
      count: 2,
      previous: false,
      next: true,
      onPrevious: vi.fn(),
      onNext: vi.fn(),
    })
    expect(container.querySelector('.error-nav__position')).toBeNull()
    expect(screen.getByText('err')).toBeInTheDocument()
  })
})
