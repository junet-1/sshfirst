// Platform detection for presentation and key handling.
//
// The webview is the only reliable source here: the same frontend bundle is
// served by every build, so nothing is known at compile time. The keyboard
// handlers themselves already accept Ctrl and Cmd interchangeably (see
// App.svelte's isCtrl); what differs per platform is how a shortcut is written
// and which of the two modifiers the terminal reserves for the shell.

function detectMac(): boolean {
  if (typeof navigator === 'undefined') return false
  const uaData = (navigator as Navigator & { userAgentData?: { platform?: string } }).userAgentData
  const platform = uaData?.platform || navigator.platform || navigator.userAgent || ''
  return /mac|iphone|ipad|ipod/i.test(platform)
}

export const isMac = detectMac()

// shortcutLabel rewrites a Ctrl-based shortcut in the notation of the running
// platform: 'Ctrl+Shift+P' reads '⌘⇧P' on macOS and is left untouched
// elsewhere.
export function shortcutLabel(label: string): string {
  return isMac ? toMacShortcut(label) : label
}

// toMacShortcut writes modifiers as the symbols macOS uses, run together
// without separators as every menu there does.
export function toMacShortcut(label: string): string {
  return label
    .replace(/\bCtrl\s*\+\s*|\bCtrl\s+/g, '⌘')
    .replace(/\bShift\s*\+\s*|\bShift\s+/g, '⇧')
    .replace(/\bAlt\s*\+\s*|\bAlt\s+/g, '⌥')
    .replace(/\bEnter\b/g, '⏎')
}
