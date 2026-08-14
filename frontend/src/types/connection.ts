export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'error'
export type TabKind = 'terminal' | 'sftp' | 'quick-connect' | 'browser' | 'connection-attempt'

export interface ConnectionInfo {
  connectionId: string
  hostId: number
  hostLabel: string
  status: ConnectionStatus
  serverVersion?: string
  authMethod?: string
  protocol: 'ssh' | 'sftp'
  initialTabId: string
  initialTabKind: TabKind
  error?: string
  warning?: string
}

export interface TabInfo {
  tabId: string
  connectionId: string
  title: string
  kind: TabKind
}

export interface SFTPEntry {
  name: string
  path: string
  isDir: boolean
  isSymlink: boolean
  size: number
  mode: string
  modifiedAt: string
}

export interface SFTPDirectory {
  path: string
  entries: SFTPEntry[]
}

export interface StatusEvent {
  connectionId: string
  hostId: number
  status: ConnectionStatus
  latencyMs?: number
  error?: string
}

export interface TerminalDataEvent {
  tabId: string
  sessionGeneration: string
  sequence: number
  data: string
}

export interface TerminalClosedEvent {
  tabId: string
  error?: string
}

export type HostKeyStatus = 'unknown' | 'changed'

export interface HostKeyPromptEvent {
  requestId: string
  hostname: string
  algorithm: string
  fingerprint: string
  status: HostKeyStatus
  previousFingerprints?: string[]
}

export interface PasswordPromptEvent {
  requestId: string
  user: string
  hostname: string
  allowRemember: boolean
}

export interface PassphrasePromptEvent {
  requestId: string
  identityFile: string
}

export interface KeyboardInteractiveQuestion {
  prompt: string
  echo: boolean
}

export interface KeyboardInteractivePromptEvent {
  requestId: string
  user: string
  hostname: string
  instruction?: string
  questions: KeyboardInteractiveQuestion[]
}
