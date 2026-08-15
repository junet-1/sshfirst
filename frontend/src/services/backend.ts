// Thin typed wrapper around the generated Wails bindings. Components must
// never import generated bindings directly — everything goes through here so the
// binding surface has one place to adapt if generated signatures change.
import * as Backend from '../../bindings/ssh-first/internal/app/app'
// @ts-ignore Wails serves this virtual module from the native asset server.
import { Browser, Events, Window as NativeWindow } from '/wails/runtime.js'
import type { Folder, Host, HostInput, ImportResult } from '../types/host'
import type { Credential, CredentialInput } from '../types/credential'
import type { ConnectionInfo, SFTPDirectory, SFTPEntry, TabInfo } from '../types/connection'
import type { Snippet, SnippetInput } from '../types/snippet'
import type { TransferRequest } from '../types/transfer'
import type { ActiveForward, ForwardRule, ForwardRuleInput } from '../types/forwarding'

export interface SettingResult {
  value: string
  exists: boolean
}

export const backend = {
  listHosts: (): Promise<Host[]> => Backend.ListHosts() as unknown as Promise<Host[]>,
  getHost: (id: number): Promise<Host> => Backend.GetHost(id) as unknown as Promise<Host>,
  createHost: (input: HostInput): Promise<Host> => Backend.CreateHost(input as any) as unknown as Promise<Host>,
  updateHost: (id: number, input: HostInput): Promise<Host> =>
    Backend.UpdateHost(id, input as any) as unknown as Promise<Host>,
  deleteHost: (id: number): Promise<void> => Backend.DeleteHost(id),
  setFavorite: (id: number, favorite: boolean): Promise<void> => Backend.SetFavorite(id, favorite),

  listFolders: (): Promise<Folder[]> => Backend.ListFolders() as unknown as Promise<Folder[]>,
  createFolder: (name: string, parentId: number | null, icon: string): Promise<Folder> =>
    Backend.CreateFolder(name, parentId as any, icon) as unknown as Promise<Folder>,
  updateFolder: (id: number, name: string, icon: string): Promise<Folder> =>
    Backend.UpdateFolder(id, name, icon) as unknown as Promise<Folder>,
  renameFolder: (id: number, name: string): Promise<void> => Backend.RenameFolder(id, name),
  deleteFolder: (id: number): Promise<void> => Backend.DeleteFolder(id),
  moveHostToFolder: (hostId: number, folderId: number | null): Promise<void> =>
    Backend.MoveHostToFolder(hostId, folderId as any),
  moveFolder: (id: number, parentId: number | null): Promise<void> =>
    Backend.MoveFolder(id, parentId as any),
  reorderHost: (hostId: number, folderId: number | null, targetHostId: number | null, before: boolean): Promise<void> =>
    Backend.ReorderHost(hostId, folderId as any, targetHostId as any, before),
  reorderFolder: (folderId: number, parentId: number | null, targetFolderId: number | null, before: boolean): Promise<void> =>
    Backend.ReorderFolder(folderId, parentId as any, targetFolderId as any, before),

  importSSHConfig: (): Promise<ImportResult> => Backend.ImportSSHConfig() as unknown as Promise<ImportResult>,

  favicon: (url: string): Promise<string> => Backend.Favicon(url),
  hostUsername: (id: number): Promise<string> => Backend.HostUsername(id),
  hostPassword: (id: number): Promise<string> => Backend.HostPassword(id),
  webPassword: (id: number): Promise<string> => Backend.WebPassword(id),

  panelViewsSupported: (): Promise<boolean> => Backend.PanelViewsSupported(),
  openPanelView: (tabId: string, url: string): Promise<void> => Backend.OpenPanelView(tabId, url),
  setPanelViewBounds: (
    tabId: string,
    x: number,
    y: number,
    width: number,
    height: number,
    viewportWidth: number,
    viewportHeight: number,
    visible: boolean
  ): Promise<void> =>
    Backend.SetPanelViewBounds(tabId, x, y, width, height, viewportWidth, viewportHeight, visible),
  closePanelView: (tabId: string): Promise<void> => Backend.ClosePanelView(tabId),
  reloadPanelView: (tabId: string): Promise<void> => Backend.ReloadPanelView(tabId),
  autofillPanelView: (tabId: string, hostId: number): Promise<void> => Backend.AutofillPanelView(tabId, hostId),
  setWebPassword: (id: number, password: string): Promise<void> => Backend.SetWebPassword(id, password),

  listCredentials: (): Promise<Credential[]> => Backend.ListCredentials() as unknown as Promise<Credential[]>,
  createCredential: (input: CredentialInput): Promise<Credential> =>
    Backend.CreateCredential(input as any) as unknown as Promise<Credential>,
  updateCredential: (id: number, input: CredentialInput): Promise<Credential> =>
    Backend.UpdateCredential(id, input as any) as unknown as Promise<Credential>,
  deleteCredential: (id: number): Promise<void> => Backend.DeleteCredential(id),

  getSetting: (key: string): Promise<SettingResult> => Backend.GetSetting(key) as unknown as Promise<SettingResult>,
  setSetting: (key: string, value: string): Promise<void> => Backend.SetSetting(key, value),

  listWorkspaces: (): Promise<string[]> => Backend.ListWorkspaces(),
  loadWorkspace: (name: string): Promise<string> => Backend.LoadWorkspace(name),
  saveWorkspace: (name: string, json: string): Promise<void> => Backend.SaveWorkspace(name, json),
  deleteWorkspace: (name: string): Promise<void> => Backend.DeleteWorkspace(name),
  importWorkspaceJSON: (): Promise<string> => Backend.ImportWorkspaceJSON(),
  exportWorkspaceJSON: (defaultFilename: string, json: string): Promise<string> =>
    Backend.ExportWorkspaceJSON(defaultFilename, json),

  openToolWindow: (kind: string, entityId = -1, parentId = -1): Promise<void> =>
    Backend.OpenToolWindow(kind, entityId, parentId),
  connectedConnectionId: (hostId: number): Promise<string> => Backend.ConnectedConnectionID(hostId),

  connect: (hostId: number, cols: number, rows: number): Promise<ConnectionInfo> =>
    Backend.Connect(hostId, cols, rows) as unknown as Promise<ConnectionInfo>,
  connectWorkspaceResource: (hostId: number, nodeType: 'terminal' | 'sftp', cols: number, rows: number): Promise<ConnectionInfo> =>
    Backend.ConnectWorkspaceResource(hostId, nodeType, cols, rows) as unknown as Promise<ConnectionInfo>,
  quickConnect: (target: string, cols: number, rows: number): Promise<ConnectionInfo> =>
    Backend.QuickConnect(target, cols, rows) as unknown as Promise<ConnectionInfo>,
  reconnect: (connectionId: string): Promise<ConnectionInfo> =>
    Backend.Reconnect(connectionId) as unknown as Promise<ConnectionInfo>,
  disconnect: (connectionId: string): Promise<void> => Backend.Disconnect(connectionId),
  newTerminalTab: (connectionId: string, cols: number, rows: number): Promise<TabInfo> =>
    Backend.NewTerminalTab(connectionId, cols, rows) as unknown as Promise<TabInfo>,
  closeTab: (tabId: string): Promise<void> => Backend.CloseTab(tabId),
  writeTerminalInput: (tabId: string, data: string): Promise<void> => Backend.WriteTerminalInput(tabId, data),
  resizeTerminal: (tabId: string, cols: number, rows: number): Promise<void> =>
    Backend.ResizeTerminal(tabId, cols, rows),
  startTerminalOutput: (tabId: string): Promise<void> => Backend.StartTerminalOutput(tabId),
  pauseTerminalOutput: (tabId: string): Promise<void> => Backend.PauseTerminalOutput(tabId),
  acknowledgeTerminalOutput: (tabId: string, sessionGeneration: string, sequence: number): Promise<void> =>
    Backend.AcknowledgeTerminalOutput(tabId, sessionGeneration, sequence),
  listSFTP: (tabId: string, remotePath: string): Promise<SFTPDirectory> =>
    Backend.ListSFTP(tabId, remotePath) as unknown as Promise<SFTPDirectory>,
  uploadSFTP: (tabId: string, localPath: string, remoteDir: string): Promise<SFTPEntry> =>
    Backend.UploadSFTP(tabId, localPath, remoteDir) as unknown as Promise<SFTPEntry>,
  downloadSFTP: (tabId: string, remotePath: string, localDir: string): Promise<string> =>
    Backend.DownloadSFTP(tabId, remotePath, localDir),
  exportTerminalOutput: (defaultFilename: string, content: string): Promise<string> =>
    Backend.ExportTerminalOutput(defaultFilename, content),
  openExternalURL: (url: string): void => {
    void Browser.OpenURL(url)
  },

  respondHostKey: (requestId: string, accept: boolean): Promise<void> => Backend.RespondHostKey(requestId, accept),
  respondPassword: (requestId: string, password: string, remember: boolean): Promise<void> =>
    Backend.RespondPassword(requestId, password, remember),
  cancelPasswordPrompt: (requestId: string): Promise<void> => Backend.CancelPasswordPrompt(requestId),
  respondPassphrase: (requestId: string, passphrase: string, remember: boolean): Promise<void> =>
    Backend.RespondPassphrase(requestId, passphrase, remember),
  cancelPassphrasePrompt: (requestId: string): Promise<void> => Backend.CancelPassphrasePrompt(requestId),
  respondKeyboardInteractive: (requestId: string, answers: string[]): Promise<void> =>
    Backend.RespondKeyboardInteractive(requestId, answers),
  cancelKeyboardInteractivePrompt: (requestId: string): Promise<void> =>
    Backend.CancelKeyboardInteractivePrompt(requestId),

  pickIdentityFiles: (): Promise<string[]> => Backend.PickIdentityFiles(),
  logFrontendError: (message: string): Promise<void> => Backend.LogFrontendError(message),
  toggleFullscreen: (): Promise<boolean> => Backend.ToggleFullscreen(),
  toggleMaximise: (): Promise<boolean> => Backend.ToggleMaximise(),

  listSnippets: (hostId: number): Promise<Snippet[]> =>
    Backend.ListSnippets(hostId) as unknown as Promise<Snippet[]>,
  createSnippet: (input: SnippetInput): Promise<Snippet> =>
    Backend.CreateSnippet(input as any) as unknown as Promise<Snippet>,
  updateSnippet: (id: number, input: SnippetInput): Promise<Snippet> =>
    Backend.UpdateSnippet(id, input as any) as unknown as Promise<Snippet>,
  deleteSnippet: (id: number): Promise<void> => Backend.DeleteSnippet(id),
  sendToTab: (tabId: string, text: string): Promise<void> => Backend.SendToTab(tabId, text),

  startRsync: (req: TransferRequest): Promise<string> => Backend.StartRsync(req as any) as unknown as Promise<string>,
  cancelRsync: (transferId: string): Promise<void> => Backend.CancelRsync(transferId),
  previewRsyncCommand: (req: TransferRequest): Promise<string> =>
    Backend.PreviewRsyncCommand(req as any) as unknown as Promise<string>,
  pickDirectory: (): Promise<string> => Backend.PickDirectory(),
  pickFile: (): Promise<string> => Backend.PickFile(),

  listForwardRules: (hostId: number): Promise<ForwardRule[]> =>
    Backend.ListForwardRules(hostId) as unknown as Promise<ForwardRule[]>,
  createForwardRule: (input: ForwardRuleInput): Promise<ForwardRule> =>
    Backend.CreateForwardRule(input as any) as unknown as Promise<ForwardRule>,
  updateForwardRule: (id: number, input: ForwardRuleInput): Promise<ForwardRule> =>
    Backend.UpdateForwardRule(id, input as any) as unknown as Promise<ForwardRule>,
  deleteForwardRule: (id: number): Promise<void> => Backend.DeleteForwardRule(id),
  startForward: (connectionId: string, ruleId: number): Promise<ActiveForward> =>
    Backend.StartForward(connectionId, ruleId) as unknown as Promise<ActiveForward>,
  stopForward: (connectionId: string, ruleId: number): Promise<void> => Backend.StopForward(connectionId, ruleId),
  listActiveForwards: (connectionId: string): Promise<ActiveForward[]> =>
    Backend.ListActiveForwards(connectionId) as unknown as Promise<ActiveForward[]>
}

/** Subscribes to a Wails runtime event; call the returned function to unsubscribe just this listener. */
export function on<T>(event: string, handler: (payload: T) => void): () => void {
  return Events.On<T>(event, ({ data }: { data: T }) => handler(data))
}

export function emit<T>(event: string, payload?: T): Promise<boolean> {
  return Events.Emit(event, payload)
}

export function closeCurrentWindow(): Promise<void> {
  return NativeWindow.Close()
}

/**
 * Resizes the current native window so its client area matches a desired
 * viewport height, keeping the current width. Tool windows use this to fit
 * their content instead of relying on hard-coded per-dialog sizes. The window's
 * MinHeight (set at creation) must be low enough not to clamp a shrink.
 */
export async function fitCurrentWindowHeight(desiredViewportHeight: number): Promise<void> {
  const size = await NativeWindow.Size()
  // Adjust the window by the same delta the viewport is off by; this stays
  // correct regardless of window chrome (menu/titlebar drawn outside the webview).
  const delta = desiredViewportHeight - window.innerHeight
  const nextHeight = Math.max(120, Math.round(size.height + delta))
  await NativeWindow.SetSize(size.width, nextHeight)
}
