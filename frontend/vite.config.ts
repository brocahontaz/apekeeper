import { defineConfig, loadEnv } from "vite";
import { fileURLToPath, URL } from "node:url";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "VITE_");
  const apiProxyTarget = env.VITE_API_PROXY_TARGET || "http://localhost:8080";

  return {
    plugins: [svelte(), tailwindcss()],
    resolve: {
      alias: {
        $lib: fileURLToPath(new URL("./src/lib", import.meta.url)),
        $components: fileURLToPath(new URL("./src/components", import.meta.url)),
        $routes: fileURLToPath(new URL("./src/routes", import.meta.url)),
      },
    },
    server: { proxy: { "/api": apiProxyTarget, "/healthz": apiProxyTarget } },
    test: { environment: "jsdom" },
  };
});
