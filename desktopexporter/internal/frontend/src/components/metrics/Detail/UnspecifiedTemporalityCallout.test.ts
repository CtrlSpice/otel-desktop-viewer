// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import UnspecifiedTemporalityCallout from './UnspecifiedTemporalityCallout.svelte'

// Whitespace is content inside the ASCII-art <pre>, so tests assert characters.

function renderFull() {
  return render(UnspecifiedTemporalityCallout, {
    props: {
      size: 'full',
      temporalityCode: 0,
      temporalityLabel: 'Unspecified',
    },
  })
}

function asciiBlocks(): string[] {
  return [...document.querySelectorAll('pre.callout-ascii')].map(
    el => el.textContent ?? ''
  )
}

describe('UnspecifiedTemporalityCallout, full', () => {
  it('renders three ascii panels', () => {
    renderFull()
    expect(asciiBlocks()).toHaveLength(3)
  })

  // HTML drops one newline after a <pre> start tag; this catches extra blank lines.
  it('starts each panel on its first line, with no blank line above the art', () => {
    renderFull()
    for (const block of asciiBlocks()) {
      expect(block.startsWith('\n')).toBe(false)
      // A panel may be indented, but its top border stays on line one.
      expect(block.split('\n')[0]).toContain('╭')
    }
  })

  // Trailing spaces align the speech-box borders.
  it('keeps the trailing spaces that pad the box edges', () => {
    renderFull()
    const padded = asciiBlocks()
      .flatMap(b => b.split('\n'))
      .filter(line => line !== line.trimEnd())
    expect(padded.length).toBeGreaterThanOrEqual(6)
  })

  it('draws each panel to a consistent width', () => {
    renderFull()
    for (const block of asciiBlocks()) {
      const lines = block.split('\n')
      const top = lines.find(l => l.includes('╭'))!
      expect(top).toMatch(/╭─+╮?/)
      expect(lines.length).toBeGreaterThan(4)
    }
  })

  // The art is one image to a screen reader, not a wall of box-drawing
  // characters read glyph by glyph.
  it('presents the vignette as a single labelled image', () => {
    renderFull()
    const img = screen.getByRole('img')
    expect(img).toHaveAttribute('aria-label')
    expect(img.getAttribute('aria-label')).toMatch(/MUST not be used/)
  })

  it('links the caption to the proto enum', () => {
    renderFull()
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute(
      'href',
      expect.stringContaining('opentelemetry-proto')
    )
  })
})

describe('UnspecifiedTemporalityCallout, mini', () => {
  it('renders a bare label with no ascii', () => {
    render(UnspecifiedTemporalityCallout, {
      props: {
        size: 'mini',
        temporalityCode: 0,
        temporalityLabel: 'Unspecified',
      },
    })
    expect(screen.getByText('unspecifiedTemporality')).toBeInTheDocument()
    expect(document.querySelectorAll('pre.callout-ascii')).toHaveLength(0)
  })

  it('says what the label means on hover', () => {
    render(UnspecifiedTemporalityCallout, {
      props: {
        size: 'mini',
        temporalityCode: 0,
        temporalityLabel: 'Unspecified',
      },
    })
    expect(screen.getByText('unspecifiedTemporality')).toHaveAttribute(
      'data-tip',
      'Temporality: unspecified'
    )
  })
})

describe('UnspecifiedTemporalityCallout, unknown codes', () => {
  it('identifies an unknown positive code without using code-0 wording', () => {
    render(UnspecifiedTemporalityCallout, {
      props: {
        size: 'full',
        temporalityCode: 99,
        temporalityLabel: 'Unknown (99)',
      },
    })

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Unknown (99)')
    expect(alert).toHaveTextContent('Received numeric code: 99')
    expect(alert).not.toHaveTextContent('MUST not be used')
    expect(alert).not.toHaveTextContent('AGGREGATION_TEMPORALITY_UNSPECIFIED')
  })

  it('identifies an unknown negative code in the mini warning', () => {
    render(UnspecifiedTemporalityCallout, {
      props: {
        size: 'mini',
        temporalityCode: -1,
        temporalityLabel: 'Unknown (-1)',
      },
    })

    const warning = screen.getByText('Unknown (-1); received code -1')
    expect(warning).toHaveAttribute(
      'data-tip',
      'Unsafe unknown temporality code -1'
    )
    expect(warning).not.toHaveTextContent('AGGREGATION_TEMPORALITY_UNSPECIFIED')
  })
})
