export const config = {
    api: {
        bodyParser: false,
    },
};

function normalizeBaseUrl(value) {
    const trimmed = String(value || '').trim();
    if (!trimmed) return '';
    if (/^https?:\/\//i.test(trimmed)) return trimmed.replace(/\/$/, '');
    return `http://${trimmed.replace(/\/$/, '')}`;
}

function buildQueryString(query, excludeKeys = []) {
    const params = new URLSearchParams();
    const excluded = new Set(excludeKeys);
    for (const [key, value] of Object.entries(query || {})) {
        if (excluded.has(key)) continue;
        if (value === undefined || value === null) continue;
        if (Array.isArray(value)) {
            for (const item of value) {
                if (item === undefined || item === null) continue;
                params.append(key, String(item));
            }
        } else {
            params.set(key, String(value));
        }
    }
    const qs = params.toString();
    return qs ? `?${qs}` : '';
}

async function readRawBody(req) {
    const chunks = [];
    for await (const chunk of req) {
        chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
    }
    return Buffer.concat(chunks);
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

    const endpoint = String(req.query?.endpoint || '').trim();
    const validEndpoints = new Set(['wallet_avatar']);
    if (!validEndpoints.has(endpoint)) {
        res.statusCode = 400;
        res.setHeader('Content-Type', 'application/json; charset=utf-8');
        res.end(JSON.stringify({ error: 'invalid endpoint' }));
        return;
    }

    const url = `${backendBaseUrl}/api/sm/${endpoint}${buildQueryString(req.query, ['endpoint'])}`;
    const fetchOpts = {
        method: req.method || 'GET',
        headers: {},
    };

    const ct = req.headers?.['content-type'];
    if (ct) fetchOpts.headers['Content-Type'] = ct;

    if (req.method === 'POST' || req.method === 'PUT' || req.method === 'PATCH') {
        const rawBody = await readRawBody(req);
        if (rawBody.length > 0) {
            fetchOpts.body = rawBody;
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
