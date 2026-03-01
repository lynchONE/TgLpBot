// Smart Money watched wallets API proxy
// Forwards requests to backend: GET/POST/PUT/DELETE /api/smart_money_watched_wallets

function normalizeBaseUrl(value) {
    const trimmed = String(value || '').trim();
    if (!trimmed) return '';
    if (/^https?:\/\//i.test(trimmed)) return trimmed.replace(/\/$/, '');
    return `http://${trimmed.replace(/\/$/, '')}`;
}

function buildQueryString(query) {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query || {})) {
        if (value === undefined || value === null) continue;
        if (Array.isArray(value)) {
            for (const item of value) {
                if (item === undefined || item === null) continue;
                params.append(key, String(item));
            }
            continue;
        }
        params.set(key, String(value));
    }
    const out = params.toString();
    return out ? `?${out}` : '';
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

    if (req.method === 'OPTIONS') {
        res.statusCode = 204;
        res.end();
        return;
    }

    const method = String(req.method || 'GET').toUpperCase();
    if (!['GET', 'POST', 'PUT', 'DELETE'].includes(method)) {
        res.statusCode = 405;
        res.setHeader('Content-Type', 'text/plain; charset=utf-8');
        res.end('method not allowed');
        return;
    }

    const hasBody = method !== 'GET';
    const url = `${backendBaseUrl}/api/smart_money_watched_wallets${!hasBody ? buildQueryString(req.query) : ''}`;

    try {
        const upstream = await fetch(url, {
            method,
            headers: hasBody ? { 'content-type': 'application/json' } : undefined,
            body: hasBody
                ? (typeof req.body === 'string' ? req.body : JSON.stringify(req.body || {}))
                : undefined,
        });
        const text = await upstream.text().catch(() => '');

        res.statusCode = upstream.status;
        res.setHeader('Cache-Control', 'no-store');

        const contentType = String(upstream.headers.get('content-type') || '');
        if (contentType.toLowerCase().includes('application/json')) {
            res.setHeader('Content-Type', contentType);
        } else {
            res.setHeader('Content-Type', 'application/json; charset=utf-8');
        }
        res.end(text || '{}');
    } catch (err) {
        res.statusCode = 502;
        res.setHeader('Content-Type', 'text/plain; charset=utf-8');
        res.end(String(err?.message || err || 'upstream fetch failed'));
    }
}
