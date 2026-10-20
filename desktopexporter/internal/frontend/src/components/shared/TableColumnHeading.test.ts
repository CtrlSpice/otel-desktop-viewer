// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import TableColumnHeading from './TableColumnHeading.svelte'

describe('TableColumnHeading', () => {
  it.each([
    ['severity', 'Log', 'Log'],
    ['body', 'Log', 'Log'],
    ['name', 'Span', 'Span'],
    ['http.method', 'span', 'Span'],
  ] as const)(
    'omits same-signal provenance for %s',
    (label, source, signal) => {
      const { container } = render(TableColumnHeading, {
        label,
        source,
        signal,
      })
      expect(screen.getByText(label)).toBeInTheDocument()
      expect(container.querySelector('.badge')).toBeNull()
    }
  )

  it.each(['Log', 'Span'] as const)(
    'places external provenance inline in the %s table',
    signal => {
      const { container } = render(TableColumnHeading, {
        label: 'service.name',
        source: 'resource',
        signal,
      })
      const heading = container.querySelector('.table-column-heading')!
      expect(heading.children[0]).toBe(screen.getByText('service.name'))
      const provenance = screen.getByRole('img', { name: 'Source: resource' })
      expect(heading.children[1]).toBe(provenance)
      expect(provenance).toHaveClass('metadata-origin', 'min-w-0', 'truncate')
      expect(provenance).not.toHaveClass('badge', 'shrink-0')
      expect(provenance).toHaveTextContent('resource')
      expect(provenance).toHaveAttribute('title', 'resource')
    }
  )
})
