// Smart Money follow configs API proxy
// Forwards requests to backend: GET /api/smart_money_follow_configs

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

    if (String(req.method || 'GET').toUpperCase() !== 'GET') {
        res.statusCode = 405;
        res.setHeader('Content-Type', 'text/plain; charset=utf-8');
        res.end('method not allowed');
        return;
    }

    const url = `${backendBaseUrl}/api/smart_money_follow_configs${buildQueryString(req.query)}`;

    try {
        const upstream = await fetch(url);
        const text = await upstream.text().catch(() => '');
        const body = String(text || '').trim();

        res.statusCode = upstream.status;
        res.setHeader('Cache-Control', 'no-store');

        const contentType = String(upstream.headers.get('content-type') || '');
        if (upstream.ok && !body) {
            res.statusCode = 200;
            res.setHeader('Content-Type', 'application/json; charset=utf-8');
            res.end(JSON.stringify({
                chain: String(req.query?.chain || 'bsc'),
                enabled_only: String(req.query?.enabled_only || '').toLowerCase() !== 'false',
                enabled_count: 0,
                configs: [],
                warnings: [`smart_money_follow_configs upstream returned empty body (HTTP ${upstream.status})`],
            }));
            return;
        }

        if (!contentType.toLowerCase().includes('application/json')) {
            res.setHeader('Content-Type', 'application/json; charset=utf-8');
            res.end(JSON.stringify({
                chain: String(req.query?.chain || 'bsc'),
                enabled_only: String(req.query?.enabled_only || '').toLowerCase() !== 'false',
                enabled_count: 0,
                configs: [],
                warnings: [body ? `smart_money_follow_configs upstream non-json body: ${body.slice(0, 200)}` : 'smart_money_follow_configs upstream non-json empty body'],
            }));
            return;
        }

        res.setHeader('Content-Type', contentType || 'application/json; charset=utf-8');
        res.end(text);
    } catch (err) {
        res.statusCode = 502;
        res.setHeader('Content-Type', 'text/plain; charset=utf-8');
        res.end(String(err?.message || err || 'upstream fetch failed'));
    }
}

