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

function appendAllowedHost(targets, raw) {
    const value = String(raw || '').trim();
    if (!value) return;

    try {
        const parsed = new URL(/^https?:\/\//i.test(value) ? value : `http://${value}`);
        if (parsed.hostname) {
            targets.add(parsed.hostname.toLowerCase());
        }
    } catch {
        // ignore invalid config values
    }
}

function isAllowedAvatarTarget(targetUrl, backendBaseUrl) {
    const value = String(targetUrl || '').trim();
    if (!value) return false;

    let parsed;
    try {
        parsed = new URL(value);
    } catch {
        return false;
    }

    if (!/^https?:$/i.test(parsed.protocol)) return false;
    if (!parsed.pathname.startsWith('/avatar/')) return false;

    const allowedHosts = new Set();
    appendAllowedHost(allowedHosts, backendBaseUrl);
    appendAllowedHost(allowedHosts, process.env.MINIO_PUBLIC_BASE_URL);

    if (allowedHosts.size === 0) return false;
    return allowedHosts.has(String(parsed.hostname || '').toLowerCase());
}

async function handleAvatarAssetProxy(req, res, backendBaseUrl) {
    if (req.method !== 'GET' && req.method !== 'HEAD') {
        res.statusCode = 405;
        res.setHeader('Content-Type', 'text/plain; charset=utf-8');
        res.end('method not allowed');
        return;
    }

    const targetUrl = String(req.query?.url || '').trim();
    if (!isAllowedAvatarTarget(targetUrl, backendBaseUrl)) {
        res.statusCode = 400;
        res.setHeader('Content-Type', 'text/plain; charset=utf-8');
        res.end('invalid avatar url');
        return;
    }

    try {
        const upstream = await fetch(targetUrl, {
            method: req.method || 'GET',
            headers: { Accept: 'image/*,*/*;q=0.8' },
        });

        res.statusCode = upstream.status;
        const contentType = upstream.headers.get('content-type');
        const contentLength = upstream.headers.get('content-length');
        const etag = upstream.headers.get('etag');
        const lastModified = upstream.headers.get('last-modified');

        if (contentType) res.setHeader('Content-Type', contentType);
        if (contentLength) res.setHeader('Content-Length', contentLength);
        if (etag) res.setHeader('ETag', etag);
        if (lastModified) res.setHeader('Last-Modified', lastModified);
        res.setHeader('Cache-Control', upstream.ok ? 'public, max-age=300' : 'no-store');

        if (req.method === 'HEAD' || upstream.status === 204 || upstream.status === 304) {
            res.end();
            return;
        }

        const body = Buffer.from(await upstream.arrayBuffer());
        res.end(body);
    } catch (err) {
        res.statusCode = 502;
        res.setHeader('Content-Type', 'text/plain; charset=utf-8');
        res.end(String(err?.message || err || 'avatar proxy failed'));
    }
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
    if (endpoint === 'avatar_asset') {
        await handleAvatarAssetProxy(req, res, backendBaseUrl);
        return;
    }

    const validEndpoints = new Set([
        'wallets',
        'wallet_zombies',
        'pool_liquidity_wallet_candidates',
        'pool_liquidity_wallet_candidates_stream',
        'pool_liquidity_wallet_import',
        'token_liquidity_wallet_candidates',
        'token_liquidity_wallet_import',
        'contracts',
        'pools',
        'pool_fee_heatmap',
        'positions',
        'position_detail',
        'events',
        'stats',
    ]);
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
        if (req.body) {
            fetchOpts.body = typeof req.body === 'string' ? req.body : JSON.stringify(req.body);
        }
    }

    try {
        const upstream = await fetch(url, fetchOpts);
        if (endpoint === 'pool_liquidity_wallet_candidates_stream') {
            res.statusCode = upstream.status;
            res.setHeader('Content-Type', upstream.headers.get('content-type') || 'text/event-stream; charset=utf-8');
            res.setHeader('Cache-Control', 'no-store');
            res.setHeader('Connection', 'keep-alive');
            res.setHeader('X-Accel-Buffering', 'no');
            if (!upstream.body) {
                res.end();
                return;
            }
            const reader = upstream.body.getReader();
            try {
                for (;;) {
                    const { value, done } = await reader.read();
                    if (done) break;
                    if (value) res.write(Buffer.from(value));
                }
            } finally {
                reader.releaseLock();
            }
            res.end();
            return;
        }
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
