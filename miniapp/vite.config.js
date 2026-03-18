import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
    plugins: [react()],
    server: {
        port:27377,
        strictPort: true,
    },
    build: {
        outDir: 'dist',
    },
});

