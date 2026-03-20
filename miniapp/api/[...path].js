function normalizeBaseUrl(value) {
    const trimmed = String(value || '').trim();
    if (!trimmed) return '';
    if (/^https?:\/\//i.test(trimmed)) return trimmed.replace(/\/$/, '');
    return `http://${trimmed.replace(/\/$/, '')}`;
}

function resolveBackendPath(req) {
    const pathParts = Array.isArray(req.query?.path)
        ? req.query.path
        : (req.query?.path ? [req.query.path] : []);
    const fromQuery = pathParts
        .map((part) => String(part || '').trim())
        .filter(Boolean)
        .join('/');
    if (fromQuery) return fromQuery;

    const rawPath = String(req.url || '').split('?')[0];
    const match = rawPath.match(/^\/api\/(.+)$/);
    if (match?.[1]) return String(match[1]).trim();

    return '';
}

export default async function handler(req, res) {
    const backendBaseUrl = normalizeBaseUrl(
        process.env.BACKEND_API_BASE_URL || process.env.VITE_API_BASE_URL,
    );
    if (!backendBaseUrl) {
        res.statusCode = 500;
        res.setHeader('Content-Type', 'text/plain; charset=utf-8');
        res.end('BACKEND_API_BASE_URL not set');
        return;
    }

    if (req.method === 'OPTIONS') {
        res.statusCode = 204;
        res.end();
        return;
    }

    const backendPath = resolveBackendPath(req);
    if (!backendPath) {
        res.statusCode = 404;
        res.end('not found');
        return;
    }

    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(req.query || {})) {
        if (key === 'path') continue;
        if (value === undefined || value === null) continue;
        if (Array.isArray(value)) {
            for (const item of value) params.append(key, String(item));
        } else {
            params.set(key, String(value));
        }
    }
    const qs = params.toString();
    const url = `${backendBaseUrl}/api/${backendPath}${qs ? `?${qs}` : ''}`;

    const fetchOpts = {
        method: req.method || 'GET',
        headers: {},
    };

    const ct = req.headers?.['content-type'];
    if (ct) fetchOpts.headers['Content-Type'] = ct;

    if (req.method === 'POST' || req.method === 'PUT' || req.method === 'PATCH') {
        if (req.body) {
            fetchOpts.body = typeof req.body === 'string' ? req.body : JSON.stringify(req.body);
        }
    }

    try {
        const upstream = await fetch(url, fetchOpts);
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
