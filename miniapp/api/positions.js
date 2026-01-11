// 合并的仓位 API: realtime_positions, me, auto_monitor, autolp_pnl_curve
// 通过 query 参数 endpoint 来区分端点
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

    if (req.method === 'OPTIONS') {
        res.statusCode = 204;
        res.end();
        return;
    }

    // 从 query 参数获取 endpoint
    const endpoint = String(req.query?.endpoint || '').trim();
    const validEndpoints = ['realtime_positions', 'me', 'auto_monitor', 'autolp_pnl_curve'];

    if (!validEndpoints.includes(endpoint)) {
        res.statusCode = 400;
        res.setHeader('Content-Type', 'application/json; charset=utf-8');
        res.end(JSON.stringify({ error: '无效的端点，有效值: realtime_positions, me, auto_monitor, autolp_pnl_curve' }));
        return;
    }

    const method = String(req.method || 'GET').toUpperCase();
    let url = `${backendBaseUrl}/api/${endpoint}`;
    let body = undefined;
    const headers = {};

    if (method === 'GET') {
        const initData = String(req.query?.initData || req.query?.init_data || '').trim();
        if (initData) {
            url += `?initData=${encodeURIComponent(initData)}`;
        }
    } else if (method === 'POST') {
        headers['content-type'] = 'application/json';
        body = typeof req.body === 'string' ? req.body : JSON.stringify(req.body || {});
    } else {
        res.statusCode = 405;
        res.setHeader('Content-Type', 'text/plain; charset=utf-8');
        res.end('method not allowed');
        return;
    }

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
