// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ThemeToggle from './ThemeToggle.svelte'

// document.documentElement persists across tests in this file, so reset the
// attribute the component owns before/after each test.
beforeEach(() => {
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-color-scheme')
  localStorage.clear()
})

afterEach(() => {
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-color-scheme')
})

describe('ThemeToggle', () => {
  it('defaults to Autumn Day when nothing is saved and the OS prefers light', () => {
    render(ThemeToggle)
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-aki-hiru'
    )
    expect(
      screen.getByRole('button', { name: 'Autumn Day theme active' })
    ).toBeInTheDocument()
    expect(localStorage.getItem('theme')).toBe('madoromi-aki-hiru')
    expect(screen.getByRole('button', { name: /^Theme:/ })).toBeInTheDocument()
    expect(
      screen.queryByRole('group', { name: 'Background' })
    ).not.toBeInTheDocument()
  })

  it('restores a saved Winter Night theme', () => {
    localStorage.setItem('theme', 'madoromi-fuyu-yoru')
    render(ThemeToggle)
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-fuyu-yoru'
    )
    expect(
      screen.getByRole('button', { name: 'Winter Night theme active' })
    ).toBeInTheDocument()
  })

  it('toggles day and night within the saved season', async () => {
    localStorage.setItem('theme', 'madoromi-aki-hiru')
    render(ThemeToggle)
    await userEvent.click(
      screen.getByRole('button', { name: 'Autumn Day theme active' })
    )
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-aki-yoru'
    )
    expect(localStorage.getItem('theme')).toBe('madoromi-aki-yoru')
    expect(document.documentElement.getAttribute('data-color-scheme')).toBe(
      'dark'
    )
    expect(
      screen.getByRole('button', { name: 'Autumn Night theme active' })
    ).toBeInTheDocument()
  })

  it('changes season while retaining the mode, then restores both', async () => {
    localStorage.setItem('theme', 'madoromi-aki-yoru')
    const view = render(ThemeToggle)
    await userEvent.click(screen.getByRole('button', { name: /^Theme:/ }))
    await userEvent.click(
      screen.getByRole('button', { name: 'Natsu (Summer) background' })
    )
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-natsu-yoru'
    )
    expect(localStorage.getItem('theme')).toBe('madoromi-natsu-yoru')
    view.unmount()
    render(ThemeToggle)
    await userEvent.click(screen.getByRole('button', { name: /^Theme:/ }))
    expect(
      screen.getByRole('button', { name: 'Natsu (Summer) background' })
    ).toHaveAttribute('aria-pressed', 'true')
    expect(
      screen.getByRole('button', { name: 'Summer Night theme active' })
    ).toHaveAttribute('aria-pressed', 'true')
  })
})
