export type AuthMethod = 'agent' | 'identity' | 'password'
export type HostSource = 'manual' | 'ssh_config'
export type HostProtocol = 'ssh' | 'sftp' | 'web'

export interface Host {
  id: number
  label: string
  hostname: string
  port: number
  user: string
  identityFiles: string[]
  proxyJump: string
  forwardAgent: boolean
  authMethod: AuthMethod
  protocol: HostProtocol
  remotePath: string
  folderId?: number
  credentialId?: number | null
  favorite: boolean
  source: HostSource
  notes: string
  loginScript: string
  controlPanelUrl: string
  tags: string[]
  lastUsedAt?: string
  createdAt: string
  updatedAt: string
}

export interface HostInput {
  label: string
  hostname: string
  port: number
  user: string
  identityFiles: string[]
  proxyJump: string
  forwardAgent: boolean
  authMethod: AuthMethod
  protocol: HostProtocol
  remotePath: string
  folderId?: number | null
  credentialId?: number | null
  notes: string
  loginScript: string
  controlPanelUrl: string
  tags: string[]
}

export interface Folder {
  id: number
  name: string
  icon: string
  parentId?: number
}

export interface ImportResult {
  importedCount: number
  hosts: Host[]
}

export function emptyHostInput(): HostInput {
  return {
    label: '',
    hostname: '',
    port: 22,
    user: '',
    identityFiles: [],
    proxyJump: '',
    forwardAgent: false,
    authMethod: 'agent',
    protocol: 'ssh',
    remotePath: '.',
    folderId: null,
    credentialId: null,
    notes: '',
    loginScript: '',
    controlPanelUrl: '',
    tags: []
  }
}

export function hostToInput(host: Host): HostInput {
  return {
    label: host.label,
    hostname: host.hostname,
    port: host.port,
    user: host.user,
    identityFiles: [...host.identityFiles],
    proxyJump: host.proxyJump,
    forwardAgent: host.forwardAgent,
    authMethod: host.authMethod,
    protocol: host.protocol ?? 'ssh',
    remotePath: host.remotePath || '.',
    folderId: host.folderId ?? null,
    credentialId: host.credentialId ?? null,
    notes: host.notes,
    loginScript: host.loginScript,
    controlPanelUrl: host.controlPanelUrl ?? '',
    tags: [...host.tags]
  }
}
