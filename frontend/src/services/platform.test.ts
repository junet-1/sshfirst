import { describe, expect, it } from 'vitest'

import { toMacShortcut } from './platform'

describe('toMacShortcut', () => {
  it('replaces modifiers with their macOS symbols', () => {
    expect(toMacShortcut('Ctrl+K')).toBe('⌘K')
    expect(toMacShortcut('Ctrl W')).toBe('⌘W')
    expect(toMacShortcut('Ctrl+Shift+P')).toBe('⌘⇧P')
    expect(toMacShortcut('Ctrl+Enter to paste')).toBe('⌘⏎ to paste')
  })

  it('leaves surrounding prose and unrelated words alone', () => {
    expect(toMacShortcut('Search hosts… (Ctrl+K)')).toBe('Search hosts… (⌘K)')
    // "Control" and "Shifting" must not be mistaken for modifier names.
    expect(toMacShortcut('Control panel')).toBe('Control panel')
    expect(toMacShortcut('Shifting the view')).toBe('Shifting the view')
  })
})
