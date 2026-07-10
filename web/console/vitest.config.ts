import { defineConfig } from "vitest/config";

// Kept separate from vite.config.ts so the app build and the test runner do not
// share plugin types across vitest's bundled Vite copy.
export default defineConfig({
  esbuild: { jsx: "automatic" },
  test: {
    globals: true,
    environment: "jsdom",
  },
});
