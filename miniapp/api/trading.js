// 交易 API: open_position, open_position_preview
// 通过 query 参数 endpoint 来区分端点
function normalizeBaseUrl(value) {
    const trimmed = String(value || '').trim();
    if (!trimmed) return '';
    if (/^https?:\/\//i.test(trimmed)) return trimmed.replace(/\/$/, '');
    return `http://${trimmed.replace(/\/$/, '')}`;
}

function buildQueryString(query, excludeKey = 'endpoint') {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query || {})) {
        if (key === excludeKey) continue;
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

    // 从 query 参数获取 endpoint
    const endpoint = String(req.query?.endpoint || '').trim();
    const validEndpoints = ['open_position', 'open_position_preview', 'open_position_prepare', 'create_pool_preview', 'create_pool_execute'];

    if (!validEndpoints.includes(endpoint)) {
        res.statusCode = 400;
        res.setHeader('Content-Type', 'application/json; charset=utf-8');
        res.end(JSON.stringify({ error: '无效的端点' }));
        return;
    }

    const method = String(req.method || 'GET').toUpperCase();

    // open_position 只支持 POST
    if (endpoint === 'open_position') {
        if (method !== 'POST') {
            res.statusCode = 405;
            res.setHeader('Content-Type', 'text/plain; charset=utf-8');
            res.end('method not allowed');
            return;
        }
        const url = `${backendBaseUrl}/api/open_position`;
        const headers = { 'content-type': 'application/json' };
        const body = typeof req.body === 'string' ? req.body : JSON.stringify(req.body || {});
        try {
            const upstream = await fetch(url, { method, headers, body });
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
        return;
    }

    // open_position_preview 只支持 POST
    if (endpoint === 'open_position_preview') {
        if (method !== 'POST') {
            res.statusCode = 405;
            res.setHeader('Content-Type', 'text/plain; charset=utf-8');
            res.end('method not allowed');
            return;
        }
        const url = `${backendBaseUrl}/api/open_position_preview`;
        const headers = { 'content-type': 'application/json' };
        const body = typeof req.body === 'string' ? req.body : JSON.stringify(req.body || {});
        try {
            const upstream = await fetch(url, { method, headers, body });
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
        return;
    }

    if (method !== 'POST') {
        res.statusCode = 405;
        res.setHeader('Content-Type', 'text/plain; charset=utf-8');
        res.end('method not allowed');
        return;
    }

    const url = `${backendBaseUrl}/api/trading?endpoint=${encodeURIComponent(endpoint)}`;
    const headers = { 'content-type': 'application/json' };
    const body = typeof req.body === 'string' ? req.body : JSON.stringify(req.body || {});
    try {
        const upstream = await fetch(url, { method, headers, body });
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
