import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: {
    rolldownOptions: {
      output: {
        // rolldown >= 1.1 hosts its runtime helpers in a shared app chunk that
        // imports http-vendor back; strict order keeps them in their own chunk.
        strictExecutionOrder: true,
        manualChunks(id) {
          if (id.includes('/node_modules/element-plus/')) return 'element-plus'
          if (id.includes('/node_modules/@element-plus/')) return 'element-icons'
          if (id.includes('/node_modules/vue') || id.includes('/node_modules/pinia') || id.includes('/node_modules/vue-router')) return 'vue-vendor'
          if (id.includes('/node_modules/axios/')) return 'http-vendor'
          if (id.includes('/node_modules/')) return 'vendor'
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/uploads': 'http://localhost:8080',
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
      },
    },
  },
})
