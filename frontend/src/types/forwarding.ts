export type ForwardKind = 'local' | 'remote' | 'dynamic'

export interface ForwardRule {
  id: number
  hostId: number
  kind: ForwardKind
  label: string
  bindAddr: string
  bindPort: number
  destHost: string
  destPort: number
  autoStart: boolean
}

export interface ForwardRuleInput {
  hostId: number
  kind: ForwardKind
  label: string
  bindAddr: string
  bindPort: number
  destHost: string
  destPort: number
  autoStart: boolean
}

export interface ActiveForward {
  ruleId: number
  kind: string
  label: string
  boundAddr: string
}

export interface ForwardStatusEvent {
  connectionId: string
  ruleId: number
  active: boolean
  kind?: string
  label?: string
  boundAddr?: string
  error?: string
}
