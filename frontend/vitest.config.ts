import { defineConfig } from 'vitest/config'

// Unit tests here cover pure, DOM-free logic (the terminal layout tree), so a
// plain node environment is enough — no Svelte plugin or jsdom needed.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts']
  }
})
