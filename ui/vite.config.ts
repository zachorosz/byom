import path from 'node:path';

import tailwindcss from '@tailwindcss/vite';
import { fileRoutes } from 'filesystem-routing/vite';
import { defineConfig } from 'vitest/config';
import solid from '@solidjs/vite-plugin';

export default defineConfig({
  // Turnkey client mode: no index.html and no mount file — the plugin
  // generates the entries around src/App.tsx, wrapped in src/Document.tsx
  // (or a built-in shell). `vite build` prerenders the shell into
  // dist/client/index.html and emits a purely static dist/client.
  plugins: [
    // `extensions` makes @solidjs/vite-plugin also compile the `?pick=` route
    // modules the fileRoutes plugin emits (their ids end in a query string).
    solid({ start: true, extensions: ['.jsx', '.tsx'] }), // add `ssr: true` for streaming SSR
    fileRoutes({ types: true }),
    // Scans source files for class names and generates their CSS into the
    // stylesheet that imports tailwindcss (src/App.css).
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@proto': path.resolve(import.meta.dirname, '../proto/gen/ts'),
    },
  },
  server: {
    port: 3000,
    proxy: {
      // Connect RPC procedure paths are "/<package>.<Service>/<Method>",
      // e.g. /library.v1.LibraryService/ListArtists. Proxying them keeps
      // the browser same-origin against the Go server in dev, matching
      // how the built SPA will be served in production.
      '^/(library|management)\\.v1\\.': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
    globals: false,
    setupFiles: ['./vitest-setup.ts'],
    // if you have few tests, try commenting this
    // out to improve performance:
    isolate: false,
  },
  build: {
    target: 'esnext',
    // Keep images as asset files instead of inlining them into the JS bundle.
    assetsInlineLimit: 0,
  },
});
