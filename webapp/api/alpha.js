import { randomUUID } from 'node:crypto';

const ALPHA_DATA_URL = 'https://alpha123.uk/api/data?fresh=1';
const ALPHA_STABILITY_URL = 'https://alpha123.uk/stability/stability_feed_v3.json';

function alphaRequestHeaders() {
  const profile = process.env.ALPHA_FETCH_PROFILE;
  if (!profile || profile === 'browser') {
    return {
      Accept: 'application/json, text/plain, */*',
      'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8',
      'Cache-Control': 'no-cache',
      Pragma: 'no-cache',
      Referer: 'https://alpha123.uk/',
      'Sec-CH-UA': '"Google Chrome";v="137", "Chromium";v="137", "Not/A)Brand";v="24"',
      'Sec-CH-UA-Mobile': '?0',
      'Sec-CH-UA-Platform': '"Windows"',
      'Sec-Fetch-Dest': 'empty',
      'Sec-Fetch-Mode': 'cors',
      'Sec-Fetch-Site': 'same-origin',
      'User-Agent':
        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36',
      'Accept-Encoding': 'gzip, deflate, br',
      Connection: 'keep-alive',
    };
  }

  if (profile === 'postman') {
    return {
      Accept: '*/*',
      'User-Agent': 'PostmanRuntime/7.51.1',
      'Accept-Encoding': 'gzip, deflate, br',
      Connection: 'keep-alive',
      'Postman-Token': randomUUID(),
    };
  }

  throw new Error(`Unsupported ALPHA_FETCH_PROFILE: ${profile}`);
}

async function fetchAlphaJson(url) {
  const response = await fetch(url, {
    method: 'GET',
    headers: alphaRequestHeaders(),
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
