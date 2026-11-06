import { describe, expect, it } from 'vitest'
import { shellQuote, toTerminal } from './paste'

describe('toTerminal', () => {
  it('types a one-line command as it is', () => {
    expect(toTerminal('df -h')).toBe('df -h')
  })
  it('wraps several lines in a bracketed paste so nothing runs', () => {
    expect(toTerminal('a\nb')).toBe('\x1b[200~a\nb\x1b[201~')
  })
})

describe('shellQuote', () => {
  it('wraps in single quotes', () => expect(shellQuote('/var/www')).toBe("'/var/www'"))
  it('survives quotes, spaces and substitutions', () => {
    expect(shellQuote("it's $(x) a b")).toBe("'it'\\''s $(x) a b'")
  })
})
