// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import RawJsonInspector from './RawJsonInspector.svelte'

describe('RawJsonInspector metadata', () => {
  it('keeps the format indicator and explains its size and provenance on hover instead of in paragraphs', () => {
    const { container } = render(RawJsonInspector, {
      active: false,
      wholeURL: '/export/logs/example',
      scopes: [{ id: 'log', label: 'Log', url: '/export/logs/example' }],
    })
    const metadata = screen.getByRole('status')
    expect(metadata).toHaveTextContent('OTLP JSON')
    expect(metadata.getAttribute('title')).toContain(
      'UTF-8 bytes before display formatting'
    )
    expect(metadata.getAttribute('title')).toContain(
      'not the original request bytes'
    )
    expect(metadata).toHaveAccessibleDescription(
      metadata.getAttribute('title')!
    )
    expect(container.querySelector('.raw-inspector__hint')).toBeNull()
    expect(container.querySelector('p')).toBeNull()
    expect(screen.queryByRole('link', { name: 'Download log' })).toBeNull()
    const folding = screen.getByRole('group', { name: 'JSON folding' })
    expect(folding.closest('.raw-inspector__controls')).not.toBeNull()
    for (const label of ['Collapse JSON', 'Expand JSON']) {
      const button = screen.getByRole('button', { name: label })
      expect(button).toHaveClass('btn-xs', 'join-item', 'chart-header-control')
      expect(button).toHaveTextContent('')
      expect(button).toHaveAttribute('data-tip', label)
      expect(button).toBeDisabled()
      expect(button.querySelector('svg')).not.toBeNull()
    }
  })

  it('retains a narrower scoped download without repeating the whole-record header export', () => {
    render(RawJsonInspector, {
      active: false,
      wholeURL: '/export/traces/example',
      scopes: [
        {
          id: 'span',
          label: 'Span',
          url: '/export/traces/example?spanId=0000000000000001',
        },
      ],
    })
    expect(screen.getByRole('link', { name: 'Download span' })).toHaveAttribute(
      'href',
      '/export/traces/example?spanId=0000000000000001'
    )
    expect(
      screen.queryByRole('link', { name: 'Download full OTLP' })
    ).toBeNull()
  })
})
