import { describe, expect, it } from 'vitest'

import { mayAutofill, normalizePanelUrl } from './panelUrl'

describe('normalizePanelUrl', () => {
  it('keeps ordinary panel URLs and assumes https for a bare host', () => {
    expect(normalizePanelUrl('https://proxmox.example.com:8006')).toBe('https://proxmox.example.com:8006/')
    expect(normalizePanelUrl('  panel.example.com:8006  ')).toBe('https://panel.example.com:8006/')
    expect(normalizePanelUrl('http://192.168.1.1/')).toBe('http://192.168.1.1/')
    expect(normalizePanelUrl('https://panel.example.com/path?a=1#x')).toBe('https://panel.example.com/path?a=1#x')
  })

  it('rejects schemes that would execute in the app shell origin', () => {
    // An iframe with a javascript: src inherits the embedder's origin, which is
    // where the Wails bindings live.
    expect(normalizePanelUrl('javascript://x%0aalert(1)')).toBe('')
    expect(normalizePanelUrl('JavaScript://x%0aalert(1)')).toBe('')
    expect(normalizePanelUrl('data://text/html,<script>alert(1)</script>')).toBe('')
    expect(normalizePanelUrl('file:///etc/passwd')).toBe('')
    expect(normalizePanelUrl('wails://wails.localhost/wails/runtime?object=0')).toBe('')
  })

  it('rejects embedded credentials, which hide the real origin', () => {
    expect(normalizePanelUrl('https://panel.corp.example@evil.tld/')).toBe('')
    expect(normalizePanelUrl('https://user:pw@evil.tld/')).toBe('')
  })

  it('rejects empty and unparseable input', () => {
    expect(normalizePanelUrl('')).toBe('')
    expect(normalizePanelUrl('   ')).toBe('')
    expect(normalizePanelUrl('https://')).toBe('')
  })
})

describe('mayAutofill', () => {
  it('allows https anywhere', () => {
    expect(mayAutofill('https://panel.example.com/')).toBe(true)
    expect(mayAutofill('https://192.168.1.1/')).toBe(true)
  })

  it('allows plaintext only where the local network is the trust boundary', () => {
    expect(mayAutofill('http://192.168.1.1/')).toBe(true)
    expect(mayAutofill('http://10.0.0.5:8006/')).toBe(true)
    expect(mayAutofill('http://172.16.4.1/')).toBe(true)
    expect(mayAutofill('http://127.0.0.1:3000/')).toBe(true)
    expect(mayAutofill('http://localhost:8080/')).toBe(true)
    expect(mayAutofill('http://fritz.box.local/')).toBe(true)
    expect(mayAutofill('http://nas.lan/')).toBe(true)
    expect(mayAutofill('http://[::1]:9000/')).toBe(true)
  })

  it('refuses plaintext on routable addresses', () => {
    expect(mayAutofill('http://panel.example.com/')).toBe(false)
    expect(mayAutofill('http://8.8.8.8/')).toBe(false)
    expect(mayAutofill('http://172.32.0.1/')).toBe(false)
    expect(mayAutofill('http://192.169.1.1/')).toBe(false)
  })

  it('refuses anything unparseable or non-http', () => {
    expect(mayAutofill('about:blank')).toBe(false)
    expect(mayAutofill('nonsense')).toBe(false)
  })
})
