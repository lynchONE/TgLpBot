import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // Increase chunk size warning limit (we expect large vendor chunks)
    chunkSizeWarningLimit: 1000,
    rollupOptions: {
      output: {
        // Manual chunks for better caching and parallel loading
        manualChunks: {
          // Vendor: React core
          'react-vendor': ['react', 'react-dom'],
          // Web3: All blockchain-related libs (heavy, ~1.5MB)
          'web3-vendor': ['wagmi', 'viem', '@rainbow-me/rainbowkit', '@tanstack/react-query'],
          // UI: Ant Design
          'ui-vendor': ['antd'],
        },
      },
    },
    // Enable minification
    minify: 'esbuild',
    // Target modern browsers only (smaller bundle)
    target: 'esnext',
  },
  // Optimize deps for faster dev startup
  optimizeDeps: {
    include: ['react', 'react-dom', 'antd'],
  },
})
