function normalizeBaseUrl(value) {
    const trimmed = String(value || '').trim();
    if (!trimmed) return '';
    if (/^https?:\/\//i.test(trimmed)) return trimmed.replace(/\/$/, '');
    return `http://${trimmed.replace(/\/$/, '')}`;
}

function isJsonContentType(value) {
    return String(value || '').toLowerCase().includes('application/json');
}

function buildQueryString(query, excludedKeys = []) {
    const excluded = new Set(excludedKeys);
    const params = new URLSearchParams();

    for (const [key, value] of Object.entries(query || {})) {
        if (excluded.has(key)) continue;
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

function normalizeRequestBody(body) {
    if (typeof body === 'string') {
        try {
            const parsed = JSON.parse(body);
            return {
                raw: body,
                parsed: parsed && typeof parsed === 'object' ? parsed : null,
            };
        } catch {
            return { raw: body, parsed: null };
        }
    }

    if (body && typeof body === 'object') {
        return { raw: JSON.stringify(body), parsed: body };
    }

    return { raw: '{}', parsed: null };
}

function setNoStore(res) {
    res.setHeader('Cache-Control', 'no-store');
}

function setJson(res) {
    res.setHeader('Content-Type', 'application/json; charset=utf-8');
}

function endMethodNotAllowed(res) {
    res.statusCode = 405;
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.end('method not allowed');
}

function endInvalidEndpoint(res) {
    res.statusCode = 400;
    setJson(res);
    res.end(JSON.stringify({ error: 'invalid endpoint' }));
}

function endFetchError(res, err) {
    res.statusCode = 502;
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.end(String(err?.message || err || 'upstream fetch failed'));
}

function getChainFromRequest(req, parsedBody) {
    const fromQuery = String(req.query?.chain || '').trim();
    if (fromQuery) return fromQuery;
    const fromBody = String(parsedBody?.chain || '').trim();
    if (fromBody) return fromBody;
    return 'bsc';
}

const ENDPOINTS = {
    smart_money: {
        methods: ['GET'],
        upstreamPath: '/api/smart_money_overview',
        queryMethods: ['GET'],
        responseMode: 'smart_money',
    },
    smart_money_pool_adds: {
        methods: ['GET'],
        upstreamPath: '/api/smart_money_pool_adds',
        queryMethods: ['GET'],
        responseMode: 'smart_money_pool_adds',
    },
    smart_money_wallet_positions: {
        methods: ['GET'],
        upstreamPath: '/api/smart_money_wallet_positions',
        queryMethods: ['GET'],
        responseMode: 'smart_money_wallet_positions',
    },
    smart_money_follow_config: {
        methods: ['GET', 'POST'],
        upstreamPath: '/api/smart_money_follow_config',
        queryMethods: ['GET'],
        bodyMethods: ['POST'],
        responseMode: 'smart_money_follow_config',
    },
    smart_money_follow_configs: {
        methods: ['GET'],
        upstreamPath: '/api/smart_money_follow_configs',
        queryMethods: ['GET'],
        responseMode: 'smart_money_follow_configs',
    },
    smart_money_golden_dog_config: {
        methods: ['GET', 'POST'],
        upstreamPath: '/api/smart_money_golden_dog_config',
        queryMethods: ['GET'],
        bodyMethods: ['POST'],
        responseMode: 'smart_money_golden_dog_config',
    },
    smart_money_watched_wallets: {
        methods: ['GET', 'POST', 'PUT', 'DELETE'],
        upstreamPath: '/api/smart_money_watched_wallets',
        queryMethods: ['GET'],
        bodyMethods: ['POST', 'PUT', 'DELETE'],
        responseMode: 'json_passthrough_or_empty_object',
    },
    smart_money_24h_pool_adds: {
        methods: ['GET'],
        upstreamPath: '/api/smart_money_24h_pool_adds',
        queryMethods: ['GET'],
        responseMode: 'json_passthrough_or_empty_object',
    },
};

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

    const endpoint = String(req.query?.endpoint || '').trim();
    const config = ENDPOINTS[endpoint];
    if (!config) {
        endInvalidEndpoint(res);
        return;
    }

    const method = String(req.method || 'GET').toUpperCase();
    if (!config.methods.includes(method)) {
        endMethodNotAllowed(res);
        return;
    }

    const includeQuery = (config.queryMethods || []).includes(method);
    const includeBody = (config.bodyMethods || []).includes(method);
    const requestBody = includeBody ? normalizeRequestBody(req.body) : { raw: undefined, parsed: null };
    const upstreamUrl = `${backendBaseUrl}${config.upstreamPath}${includeQuery ? buildQueryString(req.query, ['endpoint']) : ''}`;

    try {
        const upstream = await fetch(upstreamUrl, {
            method,
            headers: includeBody ? { 'content-type': 'application/json' } : undefined,
            body: includeBody ? requestBody.raw : undefined,
        });
        const text = await upstream.text().catch(() => '');
        const body = String(text || '').trim();
        const contentType = String(upstream.headers.get('content-type') || '');

        res.statusCode = upstream.status;
        setNoStore(res);

        if (config.responseMode === 'json_passthrough_or_empty_object') {
            if (isJsonContentType(contentType)) {
                res.setHeader('Content-Type', contentType);
            } else {
                setJson(res);
            }
            res.end(text || '{}');
            return;
        }

        if (config.responseMode === 'smart_money') {
            if (upstream.ok && !body) {
                res.statusCode = 200;
                setJson(res);
                res.end(JSON.stringify({
                    chain: String(req.query?.chain || 'bsc'),
                    pools: [],
                    wallets_24h: [],
                    summary: {},
                    warnings: [`smart_money upstream returned empty body (HTTP ${upstream.status})`],
                }));
                return;
            }

            if (!isJsonContentType(contentType)) {
                setJson(res);
                if (!body) {
                    res.end(JSON.stringify({
                        chain: String(req.query?.chain || 'bsc'),
                        pools: [],
                        wallets_24h: [],
                        summary: {},
                        warnings: ['smart_money upstream non-json empty body'],
                    }));
                    return;
                }
                res.end(JSON.stringify({
                    chain: String(req.query?.chain || 'bsc'),
                    pools: [],
                    wallets_24h: [],
                    summary: {},
                    warnings: [`smart_money upstream non-json body: ${body.slice(0, 200)}`],
                }));
                return;
            }

            res.setHeader('Content-Type', contentType || 'application/json; charset=utf-8');
            res.end(text);
            return;
        }

        if (config.responseMode === 'smart_money_pool_adds') {
            if (upstream.ok && !body) {
                res.statusCode = 200;
                setJson(res);
                res.end(JSON.stringify({
                    chain: String(req.query?.chain || 'bsc'),
                    window_sec: 0,
                    pool: {
                        pool_version: String(req.query?.pool_version || ''),
                        pool_id: String(req.query?.pool_id || ''),
                    },
                    wallets: [],
                    warnings: [`smart_money_pool_adds upstream returned empty body (HTTP ${upstream.status})`],
                }));
                return;
            }

            if (!isJsonContentType(contentType)) {
                setJson(res);
                if (!body) {
                    res.end(JSON.stringify({
                        chain: String(req.query?.chain || 'bsc'),
                        window_sec: 0,
                        pool: {
                            pool_version: String(req.query?.pool_version || ''),
                            pool_id: String(req.query?.pool_id || ''),
                        },
                        wallets: [],
                        warnings: [`smart_money_pool_adds upstream non-json empty body (HTTP ${upstream.status})`],
                    }));
                    return;
                }
                res.end(JSON.stringify({
                    chain: String(req.query?.chain || 'bsc'),
                    window_sec: 0,
                    pool: {
                        pool_version: String(req.query?.pool_version || ''),
                        pool_id: String(req.query?.pool_id || ''),
                    },
                    wallets: [],
                    warnings: [`smart_money_pool_adds upstream non-json body: ${body.slice(0, 200)}`],
                }));
                return;
            }

            res.setHeader('Content-Type', contentType || 'application/json; charset=utf-8');
            res.end(text);
            return;
        }

        if (config.responseMode === 'smart_money_wallet_positions') {
            if (upstream.ok && !body) {
                res.statusCode = 200;
                setJson(res);
                res.end(JSON.stringify({
                    chain: String(req.query?.chain || 'bsc'),
                    wallet_address: String(req.query?.wallet_address || ''),
                    positions: [],
                    warnings: [`smart_money_wallet_positions upstream returned empty body (HTTP ${upstream.status})`],
                }));
                return;
            }

            if (!isJsonContentType(contentType)) {
                setJson(res);
                if (!body) {
                    res.end(JSON.stringify({
                        chain: String(req.query?.chain || 'bsc'),
                        wallet_address: String(req.query?.wallet_address || ''),
                        positions: [],
                        warnings: [`smart_money_wallet_positions upstream non-json empty body (HTTP ${upstream.status})`],
                    }));
                    return;
                }
                res.end(JSON.stringify({
                    chain: String(req.query?.chain || 'bsc'),
                    wallet_address: String(req.query?.wallet_address || ''),
                    positions: [],
                    warnings: [`smart_money_wallet_positions upstream non-json body: ${body.slice(0, 200)}`],
                }));
                return;
            }

            res.setHeader('Content-Type', contentType || 'application/json; charset=utf-8');
            res.end(text);
            return;
        }

        if (config.responseMode === 'smart_money_follow_config') {
            if (upstream.ok && !body) {
                res.statusCode = 200;
                setJson(res);
                res.end(JSON.stringify({
                    config: {
                        chain: String(req.query?.chain || 'bsc'),
                        wallet_address: String(req.query?.wallet_address || ''),
                        enabled: false,
                        max_total_amount_usdt: 0,
                        per_trade_amount_usdt: 0,
                        delay_min_seconds: 0,
                        delay_max_seconds: 60,
                    },
                    warnings: [`smart_money_follow_config upstream returned empty body (HTTP ${upstream.status})`],
                }));
                return;
            }

            if (!isJsonContentType(contentType)) {
                setJson(res);
                res.end(JSON.stringify({
                    error: body ? `upstream non-json body: ${body.slice(0, 200)}` : 'upstream non-json empty body',
                }));
                return;
            }

            res.setHeader('Content-Type', contentType || 'application/json; charset=utf-8');
            res.end(text);
            return;
        }

        if (config.responseMode === 'smart_money_follow_configs') {
            if (upstream.ok && !body) {
                res.statusCode = 200;
                setJson(res);
                res.end(JSON.stringify({
                    chain: String(req.query?.chain || 'bsc'),
                    enabled_only: String(req.query?.enabled_only || '').toLowerCase() !== 'false',
                    enabled_count: 0,
                    configs: [],
                    warnings: [`smart_money_follow_configs upstream returned empty body (HTTP ${upstream.status})`],
                }));
                return;
            }

            if (!isJsonContentType(contentType)) {
                setJson(res);
                res.end(JSON.stringify({
                    chain: String(req.query?.chain || 'bsc'),
                    enabled_only: String(req.query?.enabled_only || '').toLowerCase() !== 'false',
                    enabled_count: 0,
                    configs: [],
                    warnings: [
                        body
                            ? `smart_money_follow_configs upstream non-json body: ${body.slice(0, 200)}`
                            : 'smart_money_follow_configs upstream non-json empty body',
                    ],
                }));
                return;
            }

            res.setHeader('Content-Type', contentType || 'application/json; charset=utf-8');
            res.end(text);
            return;
        }

        if (config.responseMode === 'smart_money_golden_dog_config') {
            if (upstream.ok && !body) {
                res.statusCode = 200;
                setJson(res);
                res.end(JSON.stringify({
                    config: {
                        chain: getChainFromRequest(req, requestBody.parsed),
                        enabled: false,
                        min_wallets: 3,
                        window_minutes: 10,
                        cooldown_minutes: 30,
                    },
                    warnings: [`smart_money_golden_dog_config upstream returned empty body (HTTP ${upstream.status})`],
                }));
                return;
            }

            if (!isJsonContentType(contentType)) {
                setJson(res);
                res.end(JSON.stringify({
                    error: body ? `upstream non-json body: ${body.slice(0, 200)}` : 'upstream non-json empty body',
                }));
                return;
            }

            res.setHeader('Content-Type', contentType || 'application/json; charset=utf-8');
            res.end(text);
            return;
        }

        res.setHeader('Content-Type', contentType || 'application/json; charset=utf-8');
        res.end(text);
    } catch (err) {
        endFetchError(res, err);
    }
}
