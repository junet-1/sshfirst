import {execFileSync} from 'node:child_process'
import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// The version shown in the About dialog is never edited by hand. Packaged
// builds pass it in: the release workflow from the pushed git tag, the PKGBUILD
// from $pkgver. Everything else derives it from the working tree, so a local
// build identifies itself as e.g. 0.2.3-4-gab12cd3-dirty rather than claiming
// to be the last release.
function resolveVersion(): string {
  if (process.env.APP_VERSION) {
    return process.env.APP_VERSION
  }
  try {
    const described = execFileSync(
      'git',
      ['describe', '--tags', '--dirty', '--always'],
      {encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore']}
    )
    return described.trim().replace(/^v/, '')
  } catch {
    // No git, or a source tarball without history.
    return '0.0.0-dev'
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  define: {
    __APP_VERSION__: JSON.stringify(resolveVersion())
  },
  build: {
    rollupOptions: {
      // Wails serves its v3 runtime from this native URL at application
      // runtime; it is intentionally not bundled by Vite.
      external: ['/wails/runtime.js']
    }
  }
})
