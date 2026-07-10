import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The console is a standalone SPA. It talks to a Polaris gateway entered at
// runtime (Settings → Connection), so there is no build-time API base URL.
export default defineConfig({
  plugins: [react()],
  server: { port: 5173 },
  build: { outDir: "dist", sourcemap: true },
});
