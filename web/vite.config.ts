import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'
import { demoApi } from './mock/demoApi'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const demo = env.VOHIVEX_DEMO === '1'
  const apiTarget = env.VITE_API_TARGET || 'http://127.0.0.1:7575'
  return {
    plugins: [vue(), ...(demo ? [demoApi()] : [])],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
      sourcemap: false,
      target: 'es2020',
      rollupOptions: {
        output: {
          manualChunks: {
            vue: ['vue', 'vue-router', 'pinia'],
            antdv: ['antdv-next'],
          },
        },
      },
    },
    server: {
      port: 18765,
      proxy: demo ? undefined : {
        '/api': apiTarget,
        '/healthz': apiTarget,
        '/metrics': apiTarget,
      },
    },
  }
})
