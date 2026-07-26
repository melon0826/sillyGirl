import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

export default defineConfig({
  base: '/',
  plugins: [vue()],
  build: {
    outDir: '../core/admin',
    emptyOutDir: true,
    assetsDir: 'assets',
    chunkSizeWarningLimit: 800,
    rollupOptions: {
      input: {
        index: 'index.html',
        home: 'home.html',
        user: 'user.html',
      },
      output: {
        manualChunks(id) {
          const moduleId = id.replace(/\\/g, '/');
          if (!moduleId.includes('node_modules')) {
            return;
          }
          if (moduleId.includes('/node_modules/vue/') || moduleId.includes('/node_modules/@vue/')) {
            return 'vue-vendor';
          }
          if (moduleId.includes('/ant-design-vue/') || moduleId.includes('/@ant-design/')) {
            return 'antd-vendor';
          }
          if (
            moduleId.includes('/@codemirror/') ||
            moduleId.includes('/@lezer/') ||
            moduleId.includes('/codemirror/')
          ) {
            return 'editor-vendor';
          }
          if (moduleId.includes('/lucide-vue-next/')) {
            return 'icons-vendor';
          }
          if (moduleId.includes('/prettier/plugins/babel')) {
            return 'format-babel';
          }
          if (moduleId.includes('/prettier/plugins/estree')) {
            return 'format-estree';
          }
          if (moduleId.includes('/prettier/')) {
            return 'format-core';
          }
          return 'vendor';
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
});
