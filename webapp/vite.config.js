import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const ALPHA_DATA_URL = 'https://alpha123.uk/api/data?fresh=1';
const ALPHA_STABILITY_URL = 'https://alpha123.uk/stability/stability_feed_v3.json';

async function fetchAlphaSource(name, url) {
  try {
    const response = await fetch(url, { headers: { Accept: 'application/json', 'User-Agent': 'curl/8.0.1' } });
    if (!response.ok) throw new Error(`Alpha ${name} HTTP ${response.status}`);
    return { name, payload: await response.json(), error: '' };
  } catch (err) {
    return { name, payload: null, error: String(err?.message || err || 'alpha fetch failed') };
  }
}

function alphaDevMiddleware() {
  return {
    name: 'alpha-dev-middleware',
    configureServer(server) {
      server.middlewares.use('/api/alpha', async (req, res) => {
        if (req.method !== 'GET') {
          res.statusCode = 405;
          res.setHeader('Content-Type', 'application/json; charset=utf-8');
          res.end(JSON.stringify({ error: 'method not allowed' }));
          return;
        }

        try {
          const results = await Promise.all([
            fetchAlphaSource('data', ALPHA_DATA_URL),
            fetchAlphaSource('stability', ALPHA_STABILITY_URL),
          ]);
          const errors = {};
          const out = {};
          results.forEach((item) => {
            if (item.error) {
              errors[item.name] = item.error;
            } else {
              out[item.name] = item.payload;
            }
          });
          if (Object.keys(errors).length > 0) out.errors = errors;
          if (!out.data && !out.stability) {
            throw new Error('all alpha sources failed');
          }
          res.statusCode = 200;
          res.setHeader('Content-Type', 'application/json; charset=utf-8');
          res.setHeader('Cache-Control', 'no-store');
          res.end(JSON.stringify(out));
        } catch (err) {
          res.statusCode = 502;
          res.setHeader('Content-Type', 'application/json; charset=utf-8');
          res.end(JSON.stringify({ error: String(err?.message || err || 'alpha fetch failed') }));
        }
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), alphaDevMiddleware()],
});
