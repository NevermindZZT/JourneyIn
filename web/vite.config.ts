import fs from 'node:fs'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const journeyinVersion = fs.readFileSync(new URL('../VERSION', import.meta.url), 'utf8').trim()
if (!journeyinVersion) throw new Error('VERSION must not be empty')

export default defineConfig({
  define: { 'import.meta.env.VITE_JOURNEYIN_VERSION': JSON.stringify(journeyinVersion) },
  plugins: [vue()],
  server: {
    host: '127.0.0.1',
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/mcp': 'http://127.0.0.1:8080',
      '/_AMapService': 'http://127.0.0.1:8080',
    },
  },
  build: { outDir: 'dist', emptyOutDir: true },
})
