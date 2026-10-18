import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Dev goes through src/backend (`npm run dev:server`) for the API proxy and login.
const server = process.env.WEB_SERVER_URL ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src/frontend", import.meta.url)) },
  },
  server: {
    proxy: {
      "/api": server,
      "/auth": server,
      "/config.json": server,
    },
  },
});
