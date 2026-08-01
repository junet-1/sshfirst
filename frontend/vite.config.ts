import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  build: {
    rollupOptions: {
      // Wails serves its v3 runtime from this native URL at application
      // runtime; it is intentionally not bundled by Vite.
      external: ['/wails/runtime.js']
    }
  }
})
