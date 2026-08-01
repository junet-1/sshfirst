export type ToolWindowKind =
  | 'host'
  | 'folder'
  | 'settings'
  | 'about'
  | 'snippets'
  | 'credentials'
  | 'transfer'
  | 'forwarding'

const params = new URLSearchParams(window.location.search)
const requestedKind = params.get('window')

export const currentToolWindowKind = (
  ['host', 'folder', 'settings', 'about', 'snippets', 'credentials', 'transfer', 'forwarding'].includes(requestedKind ?? '')
    ? requestedKind
    : null
) as ToolWindowKind | null

export const toolWindowEntityId = parseNumericParam('id')
export const toolWindowParentId = parseNumericParam('parent')

if (currentToolWindowKind) {
  document.documentElement.dataset.toolWindow = currentToolWindowKind
}

function parseNumericParam(name: string): number | null {
  const raw = params.get(name)
  if (raw == null || raw === '') return null
  const value = Number(raw)
  return Number.isSafeInteger(value) && value >= 0 ? value : null
}
