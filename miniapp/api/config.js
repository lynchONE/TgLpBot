function normalizeBaseUrl(value) {
  const trimmed = String(value || '').trim();
  if (!trimmed) return '';
  if (/^https?:\/\//i.test(trimmed)) return trimmed.replace(/\/$/, '');
  return `http://${trimmed.replace(/\/$/, '')}`;
}

export default async function handler(req, res) {
  const backendBaseUrl = normalizeBaseUrl(
    process.env.BACKEND_API_BASE_URL || process.env.VITE_API_BASE_URL,
  );
  if (!backendBaseUrl) {
    res.statusCode = 500;
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.end('BACKEND_API_BASE_URL (or VITE_API_BASE_URL) not set');
    return;
  }

  try {
    const upstream = await fetch(`${backendBaseUrl}/api/config`);
    const text = await upstream.text();

    res.statusCode = upstream.status;
    const contentType = upstream.headers.get('content-type');
    if (contentType) res.setHeader('Content-Type', contentType);
    res.setHeader('Cache-Control', 'no-store');
    res.end(text);
  } catch (err) {
    res.statusCode = 502;
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.end(String(err?.message || err || 'upstream fetch failed'));
  }
}

