import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  // Release builds pass the tag through APP_VERSION so the About dialog and the
  // packages always report the same number. Local builds show a dev marker.
  define: {
    __APP_VERSION__: JSON.stringify(process.env.APP_VERSION ?? '0.0.0-dev')
  },
  build: {
    rollupOptions: {
      // Wails serves its v3 runtime from this native URL at application
      // runtime; it is intentionally not bundled by Vite.
      external: ['/wails/runtime.js']
    }
  }
})
