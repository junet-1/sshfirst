export interface TransferRequest {
  hostId: number
  localPath: string
  remotePath: string
  upload: boolean
  archive: boolean
  compress: boolean
  delete: boolean
  dryRun: boolean
}

export interface TransferOutputEvent {
  transferId: string
  text: string
  transient: boolean
}

export interface TransferDoneEvent {
  transferId: string
  success: boolean
  error?: string
}
