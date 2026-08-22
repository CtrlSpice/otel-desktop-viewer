// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import UnspecifiedTemporalityCallout from './UnspecifiedTemporalityCallout.svelte'

// The callout is ASCII art in a <pre>, which makes it the one component in
// this tree where whitespace is content rather than layout. Nothing rendered
// it until now, so a formatter, a stray trim, or an editor stripping trailing
// spaces on save could silently misalign Lulu's telescope and no gate would
// notice -- svelte-check compiles it happily either way.
//
// The assertions below are deliberately about characters, not appearance:
// where a line begins, what sits at its end, how many lines there are. That
// is what "the art still lines up" reduces to in a test.

function renderFull() {
  return render(UnspecifiedTemporalityCallout, { props: { size: 'full' } })
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

  // Narrower than it looks, and worth stating so nobody trusts it for more
  // than it does. HTML drops a single newline immediately after a <pre> start
  // tag, so `<pre>╭` and `<pre>\n╭` reach the DOM identically -- verified by
  // mutation, which this test cannot fail on. What it does catch is a second
  // blank line, which survives that rule and pushes every panel down a row.
  it('starts each panel on its first line, with no blank line above the art', () => {
    renderFull()
    for (const block of asciiBlocks()) {
      expect(block.startsWith('\n')).toBe(false)
      // The art itself may be indented -- one panel is inset by eight spaces
      // on purpose -- so the invariant is that the top border is on line one,
      // not that the block opens with a corner.
      expect(block.split('\n')[0]).toContain('╭')
    }
  })

  // Trailing spaces pad the right-hand border of the speech boxes. They are
  // invisible in a diff and the first thing a "helpful" trim removes, which
  // would ragged the box edges.
  it('keeps the trailing spaces that pad the box edges', () => {
    renderFull()
    const padded = asciiBlocks()
      .flatMap(b => b.split('\n'))
      .filter(line => line !== line.trimEnd())
    expect(padded.length).toBeGreaterThanOrEqual(6)
  })

  // Every rule and border in a panel is drawn to one width, so the box only
  // reads as a box while the lines agree. Checking the top rule against its
  // matching bottom corner catches a line that lost or gained characters.
  it('draws each panel to a consistent width', () => {
    renderFull()
    for (const block of asciiBlocks()) {
      const lines = block.split('\n')
      const top = lines.find(l => l.includes('╭'))!
      expect(top).toMatch(/╭─+╮?/)
      // The panel is many lines of art, not a collapsed single line.
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
  // The spark slot gets a label and no art: the same fact, sized for a place
  // where a vignette would not fit.
  it('renders a bare label with no ascii', () => {
    render(UnspecifiedTemporalityCallout, { props: { size: 'mini' } })
    expect(screen.getByText('unspecifiedTemporality')).toBeInTheDocument()
    expect(document.querySelectorAll('pre.callout-ascii')).toHaveLength(0)
  })

  it('says what the label means on hover', () => {
    render(UnspecifiedTemporalityCallout, { props: { size: 'mini' } })
    expect(screen.getByText('unspecifiedTemporality')).toHaveAttribute(
      'data-tip',
      'Temporality: unspecified'
    )
  })
})
