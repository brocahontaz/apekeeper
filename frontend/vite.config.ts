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
      // Component tests mount real components, which needs the browser
      // (client-side) svelte build instead of the server one. Only set under
      // vitest: an explicit conditions list replaces Vite's defaults.
      ...(process.env.VITEST ? { conditions: ["browser"] } : {}),
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
