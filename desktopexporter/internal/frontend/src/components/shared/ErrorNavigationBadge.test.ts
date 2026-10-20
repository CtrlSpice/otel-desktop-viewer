// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import ErrorNavigationBadge from './ErrorNavigationBadge.svelte'

describe('ErrorNavigationBadge position', () => {
  it('groups the position and err label while retaining the live announcement', () => {
    const { container } = render(ErrorNavigationBadge, {
      count: 2,
      index: 1,
      previous: false,
      next: true,
      onPrevious: vi.fn(),
      onNext: vi.fn(),
    })
    const position = screen.getByText('[1/2 err]')
    expect(position).toHaveClass('error-nav__position')
    expect(position).toHaveAttribute('aria-live', 'polite')
    expect(container.querySelector('.error-nav__dot')).toHaveAttribute(
      'aria-hidden',
      'true'
    )
  })

  it('shows the total without a position when no error is selected', () => {
    render(ErrorNavigationBadge, {
      count: 2,
      previous: false,
      next: true,
      onPrevious: vi.fn(),
      onNext: vi.fn(),
    })
    expect(screen.getByText('[2 err]')).toBeInTheDocument()
  })
})
