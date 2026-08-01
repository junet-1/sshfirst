export interface Snippet {
  id: number
  name: string
  command: string
  hostId?: number
}

export interface SnippetInput {
  name: string
  command: string
  hostId?: number | null
}
