import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import type { Plugin } from 'vite'
import { defineConfig } from 'vitest/config'

// Everything the app needs ships inside it, so the window may load nothing from anywhere else and run no
// injected script. Only added to production builds: the dev server injects inline scripts for hot reload.
// Set SSHDECK_NO_CSP=1 when building to leave it out (if a packaged window comes up blank, try that first).
const csp = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data:",
  "font-src 'self' data:",
  "connect-src 'self'",
  "worker-src 'self'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
].join('; ')

const securityPolicy = (): Plugin => ({
  name: 'sshdeck-csp',
  apply: 'build',
  transformIndexHtml: (html) => (process.env.SSHDECK_NO_CSP ? html : html.replace('<head>', `<head>\n    <meta http-equiv="Content-Security-Policy" content="${csp}" />`)),
})

export default defineConfig({
  plugins: [react(), tailwindcss(), securityPolicy()],
  test: { environment: 'node' },
})
