import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const orchestrator = process.env.DBX_ORCHESTRATOR_URL || 'http://127.0.0.1:8000'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api': {
        target: orchestrator,
        secure: false,
      },
      '/t': {
        target: orchestrator,
        secure: false,
      }
    }
  }
})
