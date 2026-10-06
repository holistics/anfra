import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

// `pnpm build` writes the Shell where the Go binary embeds it (../assets).
export default defineConfig({
  base: '/_anfra/',
  plugins: [vue()],
  build: {
    outDir: '../assets/shell/dist',
    emptyOutDir: true,
  },
});
