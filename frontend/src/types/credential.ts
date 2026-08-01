import type { AuthMethod } from './host'

export interface Credential {
  id: number
  name: string
  user: string
  authMethod: AuthMethod
  identityFiles: string[]
  createdAt: string
  updatedAt: string
}

export interface CredentialInput {
  name: string
  user: string
  authMethod: AuthMethod
  identityFiles: string[]
}

export function emptyCredentialInput(): CredentialInput {
  return { name: '', user: '', authMethod: 'agent', identityFiles: [] }
}

export function credentialToInput(credential: Credential): CredentialInput {
  return {
    name: credential.name,
    user: credential.user,
    authMethod: credential.authMethod,
    identityFiles: [...credential.identityFiles]
  }
}
