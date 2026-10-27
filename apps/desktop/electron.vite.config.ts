import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'electron-vite';

// Workspace packages ship TypeScript sources, so they are bundled rather than left as runtime imports.
const workspace = [
  '@studio/agents',
  '@studio/db',
  '@studio/engine',
  '@studio/git',
  '@studio/prompts',
  '@studio/schema',
];

export default defineConfig({
  main: {
    build: { externalizeDeps: { exclude: workspace } },
  },
  preload: {
    // Sandboxed preload scripts must be CommonJS.
    build: {
      externalizeDeps: { exclude: workspace },
      rolldownOptions: { output: { format: 'cjs', entryFileNames: '[name].cjs' } },
    },
  },
  renderer: {
    plugins: [react(), tailwindcss()],
  },
});
