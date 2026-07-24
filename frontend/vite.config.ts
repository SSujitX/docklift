import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";
import packageJson from "./package.json" with { type: "json" };

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const port = Number(env.PORT || 3600);

  return {
    plugins: [react()],
    define: {
      __APP_VERSION__: JSON.stringify(packageJson.version),
    },
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      port,
      strictPort: true,
      host: "127.0.0.1",
      proxy: {
        "/api": {
          target: "http://127.0.0.1:8000",
          changeOrigin: true,
          configure: (proxy) => {
            proxy.on("proxyReq", (proxyReq, req) => {
              // Keep cookie + GitHub setup_url on the Vite origin (not :8000)
              if (req.headers.host) {
                proxyReq.setHeader("X-Forwarded-Host", req.headers.host);
              }
              proxyReq.setHeader("X-Forwarded-Proto", "http");
            });
          },
        },
        "/ws": {
          target: "ws://127.0.0.1:8000",
          ws: true,
        },
      },
    },
    build: {
      outDir: "dist",
      sourcemap: false,
    },
  };
});
