import { resolve } from 'path'
import { defineConfig } from 'electron-vite'
import react from '@vitejs/plugin-react'

// electron-vite drives three bundles from one config: main, preload, renderer.
// The renderer is a normal Vite + React SPA; main and preload are Node/Electron.
export default defineConfig({
  main: {
    build: {
      rollupOptions: {
        // `ws` is CJS with optional native deps we don't ship — keep it external
        // so it's required from node_modules at runtime rather than bundled.
        external: ['ws'],
      },
    },
  },
  preload: {},
  renderer: {
    root: 'src/renderer',
    resolve: {
      alias: {
        '@': resolve('src/renderer/src'),
      },
    },
    build: {
      rollupOptions: {
        input: {
          index: resolve('src/renderer/index.html'),
        },
      },
    },
    plugins: [react()],
  },
})
