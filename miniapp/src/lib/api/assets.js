import { readErrorDetails } from './request';

const ASSET_CACHE_TTL_MS = 60_000;
const assetResponseCache = new Map();

function cloneCachedPayload(payload) {
    if (payload === null || payload === undefined) return payload;
    try {
        return JSON.parse(JSON.stringify(payload));
    } catch {
        return payload;
    }
}

function readAssetCache(cacheKey, ttlMs) {
    if (!cacheKey || ttlMs <= 0) return undefined;
    const entry = assetResponseCache.get(cacheKey);
    if (!entry) return undefined;
    if (entry.expiresAt <= Date.now()) {
        assetResponseCache.delete(cacheKey);
        return undefined;
    }
    return cloneCachedPayload(entry.payload);
}

function writeAssetCache(cacheKey, payload, ttlMs) {
    if (!cacheKey || ttlMs <= 0) return;
    assetResponseCache.set(cacheKey, {
        payload: cloneCachedPayload(payload),
        expiresAt: Date.now() + ttlMs,
    });
}

async function resolveAssetCachedPayload({ cacheKey, ttlMs = ASSET_CACHE_TTL_MS, forceRefresh = false, load }) {
    if (!forceRefresh) {
        const cached = readAssetCache(cacheKey, ttlMs);
        if (cached !== undefined) return cached;
    }
    const payload = await load();
    writeAssetCache(cacheKey, payload, ttlMs);
    return cloneCachedPayload(payload);
}

export async function fetchAdminSmartMoneyOverview({
    apiBaseUrl,
    initData,
    days = 7,
    page = 1,
    pageSize = 10,
    keyword = '',
    section = '',
    sections,
    forceRefresh = false,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=assets_smart_money_overview`;
    const normalizedKeyword = String(keyword || '').trim().toLowerCase();
    const normalizedSections = Array.isArray(sections)
        ? sections.map((item) => String(item || '').trim().toLowerCase()).filter(Boolean)
        : [];
    const sectionKey = [String(section || '').trim().toLowerCase(), ...normalizedSections].filter(Boolean).join(',') || 'all';
    const cacheKey = `admin-smart-money-overview:${base}:${initData}:${sectionKey}:${days}:${page}:${pageSize}:${normalizedKeyword}`;
    return resolveAssetCachedPayload({
        cacheKey,
        forceRefresh,
        load: async () => {
            const body = {
                initData,
                days,
                page,
                page_size: pageSize,
                keyword: normalizedKeyword,
                force_refresh: forceRefresh,
            };
            if (String(section || '').trim()) body.section = String(section || '').trim().toLowerCase();
            if (normalizedSections.length) body.sections = normalizedSections;
            const resp = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
                signal,
            });
            if (!resp.ok) {
                const text = await resp.text().catch(() => '');
                throw new Error(text || `HTTP ${resp.status}`);
            }
            const payload = await resp.json();
            return payload?.data ?? payload;
        },
    });
}

export async function fetchAdminSmartMoneyWallet({ apiBaseUrl, initData, address, chainId, days = 7, forceRefresh = false, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=assets_smart_money_wallet`;
    const cacheKey = `admin-smart-money-wallet:${base}:${initData}:${String(address || '').toLowerCase()}:${chainId}:${days}`;
    return resolveAssetCachedPayload({
        cacheKey,
        forceRefresh,
        load: async () => {
            const resp = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ initData, address, chain_id: chainId, days, force_refresh: forceRefresh }),
                signal,
            });
            if (!resp.ok) {
                const text = await resp.text().catch(() => '');
                throw new Error(text || `HTTP ${resp.status}`);
            }
            const payload = await resp.json();
            return payload?.data ?? payload;
        },
    });
}

export async function fetchAdminSmartMoneyLeaderboard({
    apiBaseUrl,
    initData,
    days = 1,
    metric = 'pnl',
    page = 1,
    pageSize = 10,
    keyword = '',
    forceRefresh = false,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=assets_smart_money_leaderboard`;
    const cacheKey = `admin-smart-money-leaderboard:${base}:${initData}:${days}:${metric}:${page}:${pageSize}:${keyword}`;
    return resolveAssetCachedPayload({
        cacheKey,
        forceRefresh,
        load: async () => {
            const resp = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ initData, days, metric, page, page_size: pageSize, keyword, force_refresh: forceRefresh }),
                signal,
            });
            if (!resp.ok) {
                const text = await resp.text().catch(() => '');
                throw new Error(text || `HTTP ${resp.status}`);
            }
            const payload = await resp.json();
            return payload?.data ?? payload;
        },
    });
}

export async function fetchAssetOverview({ apiBaseUrl, initData, forceRefresh = false, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=assets_overview`;
    const cacheKey = `asset-overview:${base}:${initData}`;
    return resolveAssetCachedPayload({
        cacheKey,
        forceRefresh,
        load: async () => {
            const resp = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ initData, force_refresh: forceRefresh }),
                signal,
            });
            if (!resp.ok) {
                const text = await resp.text().catch(() => '');
                throw new Error(text || `HTTP ${resp.status}`);
            }
            const payload = await resp.json();
            return payload?.data ?? payload;
        },
    });
}

export async function fetchAssetHistory({ apiBaseUrl, initData, days = 30, forceRefresh = false, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=assets_history`;
    const cacheKey = `asset-history:${base}:${initData}:${days}`;
    return resolveAssetCachedPayload({
        cacheKey,
        forceRefresh,
        load: async () => {
            const resp = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ initData, days, force_refresh: forceRefresh }),
                signal,
            });
            if (!resp.ok) {
                const text = await resp.text().catch(() => '');
                throw new Error(text || `HTTP ${resp.status}`);
            }
            const payload = await resp.json();
            return payload?.data ?? payload;
        },
    });
}

export async function fetchAssetLPStats({ apiBaseUrl, initData, forceRefresh = false, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=assets_lp_stats`;
    const cacheKey = `asset-lp:${base}:${initData}`;
    return resolveAssetCachedPayload({
        cacheKey,
        forceRefresh,
        load: async () => {
            const resp = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ initData, force_refresh: forceRefresh }),
                signal,
            });
            if (!resp.ok) {
                const text = await resp.text().catch(() => '');
                throw new Error(text || `HTTP ${resp.status}`);
            }
            const payload = await resp.json();
            return payload?.data ?? payload;
        },
    });
}

export async function saveAssetLPPnLAdjustment({ apiBaseUrl, initData, day, manualAdjustmentUsd, note = '', signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=assets_lp_pnl_adjustment`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            initData,
            day,
            manual_adjustment_usd: manualAdjustmentUsd,
            note,
        }),
        signal,
    });
    if (!resp.ok) {
        const detail = await readErrorDetails(resp);
        const err = new Error(detail.message);
        err.status = resp.status;
        if (detail.payload && typeof detail.payload === 'object') {
            err.payload = detail.payload;
            Object.assign(err, detail.payload);
        }
        throw err;
    }
    assetResponseCache.delete(`asset-lp:${base}:${initData}`);
    const payload = await resp.json();
    return payload?.data ?? payload;
}

export async function clearAssetLPPnLAdjustment({ apiBaseUrl, initData, day, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=assets_lp_pnl_adjustment`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            initData,
            day,
            clear: true,
            action: 'clear',
        }),
        signal,
    });
    if (!resp.ok) {
        const detail = await readErrorDetails(resp);
        const err = new Error(detail.message);
        err.status = resp.status;
        if (detail.payload && typeof detail.payload === 'object') {
            err.payload = detail.payload;
            Object.assign(err, detail.payload);
        }
        throw err;
    }
    assetResponseCache.delete(`asset-lp:${base}:${initData}`);
    const payload = await resp.json();
    return payload?.data ?? payload;
}

export async function saveAssetLPPnLBaseline({ apiBaseUrl, initData, day, basePnlUsd, note = '', signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=assets_lp_pnl_baseline`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            initData,
            day,
            base_pnl_usd: basePnlUsd,
            note,
        }),
        signal,
    });
    if (!resp.ok) {
        const detail = await readErrorDetails(resp);
        const err = new Error(detail.message);
        err.status = resp.status;
        if (detail.payload && typeof detail.payload === 'object') {
            err.payload = detail.payload;
            Object.assign(err, detail.payload);
        }
        throw err;
    }
    assetResponseCache.delete(`asset-lp:${base}:${initData}`);
    const payload = await resp.json();
    return payload?.data ?? payload;
}

export async function clearAssetLPPnLBaseline({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=assets_lp_pnl_baseline`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            initData,
            clear: true,
            action: 'clear',
        }),
        signal,
    });
    if (!resp.ok) {
        const detail = await readErrorDetails(resp);
        const err = new Error(detail.message);
        err.status = resp.status;
        if (detail.payload && typeof detail.payload === 'object') {
            err.payload = detail.payload;
            Object.assign(err, detail.payload);
        }
        throw err;
    }
    assetResponseCache.delete(`asset-lp:${base}:${initData}`);
    const payload = await resp.json();
    return payload?.data ?? payload;
}

