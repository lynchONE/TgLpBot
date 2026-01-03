export async function fetchRealtimePositions({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/realtime_positions`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminRealtimeUsers({ apiBaseUrl, initData, limit, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin/realtime_users`;
    const payload = { initData };
    if (Number.isFinite(limit)) payload.limit = limit;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminRealtimePositions({ apiBaseUrl, initData, userId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin/realtime_positions`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, userId }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminAutoLPStats({ apiBaseUrl, initData, userId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin/autolp_stats`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, userId }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function disableAdminAutoLP({ apiBaseUrl, initData, userId, reason, gasMultiplier, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin/autolp_disable`;
    const payload = { initData, userId };
    if (reason) payload.reason = String(reason);
    if (Number.isFinite(gasMultiplier)) payload.gasMultiplier = gasMultiplier;

    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchHotPools({ apiBaseUrl, sort, chain, timeframeMinutes, limit, dex, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (sort) params.set('sort', String(sort));
    if (chain) params.set('chain', String(chain));
    if (Number.isFinite(timeframeMinutes)) params.set('timeframe_minutes', String(timeframeMinutes));
    if (Number.isFinite(limit)) params.set('limit', String(limit));
    if (dex) params.set('dex', String(dex));

    const qs = params.toString();
    const url = `${base}/api/hot_pools${qs ? `?${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchPoolOHLCV({ apiBaseUrl, chain, poolAddress, timeframe, aggregate, limit, beforeTimestamp, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (chain) params.set('chain', String(chain));
    if (poolAddress) params.set('pool_address', String(poolAddress));
    if (timeframe) params.set('timeframe', String(timeframe));
    if (Number.isFinite(aggregate)) params.set('aggregate', String(aggregate));
    if (Number.isFinite(limit)) params.set('limit', String(limit));
    if (Number.isFinite(beforeTimestamp)) params.set('before_timestamp', String(beforeTimestamp));

    const qs = params.toString();
    const url = `${base}/api/pool_ohlcv${qs ? `?${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}
