import { describe, expect, it } from 'vitest'
import { shellTokens } from './shell-highlighting'

describe('shell highlighting', () => {
  it.each([
    'printf "%s\\n" "$HOME" | head -1',
    '# comment\nfor file in *.json; do\n  cat "$file"\ndone',
    'echo "incomplete',
    "cat <<'EOF'\n<untrusted>&\"\nEOF\n",
    '\tprintf "λ🙂"\r\n',
  ])('keeps every original character in %j', text => {
    expect(
      shellTokens(text)
        .map(token => token.text)
        .join('')
    ).toBe(text)
  })

  it('uses shared syntax classes for shell comments and keywords', () => {
    const tokens = shellTokens('# comment\nif true; then echo "$HOME"; fi')
    expect(tokens.find(token => token.text === '# comment')?.classes).not.toBe(
      ''
    )
    expect(tokens.find(token => token.text === 'if')?.classes).not.toBe('')
  })
})
