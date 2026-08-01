import './styles/global.css'
import App from './App.svelte'
import { backend } from './services/backend'

// Otherwise-invisible webview crashes (uncaught exceptions, rejected
// promises) get forwarded into the Go log at
// ~/.local/share/ssh-first/ssh-first.log, since there is no devtools console
// in a production build.
window.addEventListener('error', (event) => {
  void backend.logFrontendError(`${event.message} (${event.filename}:${event.lineno}:${event.colno})\n${event.error?.stack ?? ''}`)
})
window.addEventListener('unhandledrejection', (event) => {
  const reason = event.reason
  const detail = reason instanceof Error ? (reason.stack ?? reason.message) : String(reason)
  void backend.logFrontendError(`unhandled rejection: ${detail}`)
})

const app = new App({
  target: document.getElementById('app') as HTMLElement
})

export default app
