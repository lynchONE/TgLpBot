import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
    plugins: [react()],
    server: {
        port: 60177,
        strictPort: true,
    },
    build: {
        outDir: 'dist',
    },
});

