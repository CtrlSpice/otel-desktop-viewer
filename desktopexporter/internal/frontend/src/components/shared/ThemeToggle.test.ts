// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ThemeToggle from './ThemeToggle.svelte'

// document.documentElement persists across tests in this file, so reset the
// attribute the component owns before/after each test.
beforeEach(() => {
  document.documentElement.removeAttribute('data-theme')
})

afterEach(() => {
  document.documentElement.removeAttribute('data-theme')
})

describe('ThemeToggle', () => {
  it('defaults to the dawn theme when nothing is saved and the OS prefers light', () => {
    render(ThemeToggle)
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'rose-pine-dawn'
    )
    expect(
      screen.getByRole('button', { name: 'Dawn theme active' })
    ).toBeInTheDocument()
    expect(localStorage.getItem('theme')).toBe('rose-pine-dawn')
  })

  it('restores a previously saved theme from localStorage', () => {
    localStorage.setItem('theme', 'rose-pine')
    render(ThemeToggle)
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'rose-pine'
    )
    expect(
      screen.getByRole('button', { name: 'Pine theme active' })
    ).toBeInTheDocument()
  })

  it('cycles to the next theme and persists it when clicked', async () => {
    render(ThemeToggle)
    await userEvent.click(screen.getByRole('button'))
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'rose-pine-moon'
    )
    expect(localStorage.getItem('theme')).toBe('rose-pine-moon')
    expect(
      screen.getByRole('button', { name: 'Moon theme active' })
    ).toBeInTheDocument()
  })

  it('wraps back to the dawn theme after cycling through all themes', async () => {
    render(ThemeToggle)
    const button = screen.getByRole('button')
    await userEvent.click(button)
    await userEvent.click(button)
    await userEvent.click(button)
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'rose-pine-dawn'
    )
    expect(localStorage.getItem('theme')).toBe('rose-pine-dawn')
  })
})
