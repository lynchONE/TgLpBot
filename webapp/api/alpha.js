const ALPHA_DATA_URL = 'https://alpha123.uk/api/data?fresh=1';
const ALPHA_STABILITY_URL = 'https://alpha123.uk/stability/stability_feed_v3.json';

async function fetchAlphaJson(url) {
  const response = await fetch(url, {
    method: 'GET',
    headers: {
      Accept: 'application/json',
      'User-Agent': 'Mozilla/5.0',
    },
  });
  if (!response.ok) {
    throw new Error(`Alpha upstream HTTP ${response.status}`);
  }
  return response.json();
}

async function fetchAlphaSource(name, url) {
  try {
    return { name, payload: await fetchAlphaJson(url), error: '' };
  } catch (err) {
    return { name, payload: null, error: String(err?.message || err || 'alpha fetch failed') };
  }
}

export default async function handler(req, res) {
  if (req.method === 'OPTIONS') {
    res.statusCode = 204;
    res.end();
    return;
  }

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
    res.setHeader('Cache-Control', 's-maxage=30, stale-while-revalidate=60');
    res.end(JSON.stringify(out));
  } catch (err) {
    res.statusCode = 502;
    res.setHeader('Content-Type', 'application/json; charset=utf-8');
    res.setHeader('Cache-Control', 'no-store');
    res.end(JSON.stringify({ error: String(err?.message || err || 'alpha fetch failed') }));
  }
}
