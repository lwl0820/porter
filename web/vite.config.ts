import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  server: { proxy: { "/api": "http://127.0.0.1:8080" } },
  build: { sourcemap: false, rolldownOptions: { input: { admin: "admin/index.html", user: "user/index.html" } } },
  test: { environment: "jsdom", clearMocks: true, restoreMocks: true },
});
