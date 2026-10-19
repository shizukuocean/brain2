import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  root: 'launcher',
  publicDir: '../public',
  plugins: [vue()],
  build: {
    target: 'es2022',
    outDir: '../../launcherfrontend/dist',
    emptyOutDir: true,
  },
})
