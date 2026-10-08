import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

// Built (`pnpm build`), it lands where the anfra binary embeds it, and anfra serve serves it below
// /appserve/. Under `vite dev` it is served at the root with hot reload, and everything else goes to
// a running `anfra serve` (ANFRA_SERVE_URL, else the default address).
const serve = process.env.ANFRA_SERVE_URL || 'http://127.0.0.1:7878';
// Where the dev server listens: make dev's anfra serve redirects its pages here (the Makefile sets
// it), so it must not move to another port when this one is taken.
const dev = new URL(process.env.ANFRA_APPSERVE_DEV_URL || 'http://127.0.0.1:5173');

export default defineConfig(({ command }) => ({
  base: command === 'build' ? '/appserve/' : '/',
  plugins: [vue()],
  build: {
    outDir: '../../internal/appserve/dist',
    emptyOutDir: true,
  },
  server: {
    host: dev.hostname,
    port: Number(dev.port) || 5173,
    strictPort: true,
    proxy: {
      '/api': serve,
      '/appserve': serve,
    },
  },
}));
