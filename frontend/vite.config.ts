import react from '@vitejs/plugin-react'
import basicSsl from '@vitejs/plugin-basic-ssl'
import { defineConfig } from 'vite'

const piURL = process.env.VITE_PI_URL ?? 'http://192.168.2.91:8081'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), basicSsl()],
  server: {
    host: true,
    proxy: {
      '/api': { target: piURL, ws: true, secure: false },
    },
  },
})
