import { defineConfig } from 'vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'

// In production the Go server serves the SPA from the same origin, so the
// frontend can use relative URLs. In dev, Vite is on :5173 and the Go
// server is on :3000 — proxy /api and /api/stream over so the frontend
// can keep using relative URLs in both modes.
export default defineConfig({
  plugins: [
    react(),
    babel({ presets: [reactCompilerPreset()] })
  ],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:3000',
        changeOrigin: true,
        ws: true, // forwards /api/stream WebSocket
      },
    },
  },
})
