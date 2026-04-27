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

async function readErrorDetails(resp) {
    const text = await resp.text().catch(() => '');
    if (!text) {
        return { message: `HTTP ${resp.status}`, payload: null };
    }
    try {
        const parsed = JSON.parse(text);
        if (parsed && typeof parsed === 'object') {
            return {
                message: parsed?.message ? String(parsed.message) : `HTTP ${resp.status}`,
                payload: parsed,
            };
        }
    } catch {
        // ignore JSON parse errors
    }
    return { message: text, payload: null };
}

export async function fetchRealtimePositions({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=realtime_positions`;
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

export async function fetchMe({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=me`;
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

export async function setTaskPaused({ apiBaseUrl, initData, taskId, paused, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=pause`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, taskId, paused: Boolean(paused) }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function stopTask({ apiBaseUrl, initData, taskId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=stop`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, taskId }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function deleteTask({ apiBaseUrl, initData, taskId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=delete`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, taskId }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function updateTaskRange({ apiBaseUrl, initData, taskId, rangeLowerPct, rangeUpperPct, amountUSDT, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=update_range`;
    const payload = {
        initData,
        taskId,
        range_lower_pct: rangeLowerPct,
        range_upper_pct: rangeUpperPct,
    };
    const amount = Number(amountUSDT);
    if (Number.isFinite(amount) && amount > 0) {
        payload.amount_usdt = amount;
    }
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

export async function withdrawLiquidity({ apiBaseUrl, initData, taskId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=withdraw_liquidity`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, taskId }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function swapDust({ apiBaseUrl, initData, taskId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=swap_dust`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, taskId }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function triggerRebalance({ apiBaseUrl, initData, taskId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=trigger_rebalance`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, taskId }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function toggleRebalance({ apiBaseUrl, initData, taskId, rebalanceEnabled, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=toggle_rebalance`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, taskId, rebalanceEnabled: Boolean(rebalanceEnabled) }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function updateTaskMode({ apiBaseUrl, initData, taskId, taskMode, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=update_mode`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, taskId, taskMode: String(taskMode || '').trim() }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function addLiquidity({ apiBaseUrl, initData, taskId, amountUsdt, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=add_liquidity`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, taskId, amountUsdt: Number(amountUsdt) }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchGlobalConfig({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/settings?endpoint=global_config`;
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

export async function fetchWallets({ apiBaseUrl, initData, chain, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/settings?endpoint=wallets`;
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        let detail = text;
        try {
            const parsed = text ? JSON.parse(text) : null;
            if (parsed?.message) detail = parsed.message;
        } catch {
            // ignore JSON parse
        }
        throw new Error(detail || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminRealtimeUsers({ apiBaseUrl, initData, limit, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=realtime_users`;
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
    const url = `${base}/api/admin?endpoint=realtime_positions`;
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

export async function fetchAdminSmartMoneyOverview({
    apiBaseUrl,
    initData,
    days = 7,
    page = 1,
    pageSize = 10,
    keyword = '',
    forceRefresh = false,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=assets_smart_money_overview`;
    const normalizedKeyword = String(keyword || '').trim().toLowerCase();
    const cacheKey = `admin-smart-money-overview:${base}:${initData}:${days}:${page}:${pageSize}:${normalizedKeyword}`;
    return resolveAssetCachedPayload({
        cacheKey,
        forceRefresh,
        load: async () => {
            const resp = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    initData,
                    days,
                    page,
                    page_size: pageSize,
                    keyword: normalizedKeyword,
                    force_refresh: forceRefresh,
                }),
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

export async function fetchAdminOnlineUsers({ apiBaseUrl, initData, limit, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=online_users`;
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

export async function fetchAdminActiveTasks({ apiBaseUrl, initData, limit, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=active_tasks`;
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

export async function fetchAdminUserAccess({ apiBaseUrl, initData, userId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams({ endpoint: 'user_access' });
    if (initData) params.set('initData', String(initData));
    if (userId) params.set('userId', String(userId));
    const url = `${base}/api/admin?${params.toString()}`;
    const resp = await fetch(url, { method: 'GET', signal });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchHotPools({ apiBaseUrl, initData, sort, chain, timeframeMinutes, limit, dex, includePools, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (sort) params.set('sort', String(sort));
    if (chain) params.set('chain', String(chain));
    if (Number.isFinite(timeframeMinutes)) params.set('timeframe_minutes', String(timeframeMinutes));
    if (Number.isFinite(limit)) params.set('limit', String(limit));
    if (dex) params.set('dex', String(dex));
    // 添加 include_pools 参数（逗号分隔的池子地址列表）
    if (Array.isArray(includePools) && includePools.length > 0) {
        params.set('include_pools', includePools.join(','));
    }

    const qs = params.toString();
    const url = `${base}/api/pools${qs ? `?${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchSearchPools({ apiBaseUrl, initData, q, chain, limit, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (q) params.set('q', String(q));
    if (chain) params.set('chain', String(chain));
    if (Number.isFinite(limit)) params.set('limit', String(limit));

    const qs = params.toString();
    const url = `${base}/api/pools?endpoint=search_pools${qs ? `&${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

function buildOpenPositionPayload({
    initData,
    chain,
    poolAddress,
    poolVersion,
    amount,
    rangeInputMode,
    rangeLowerPct,
    rangeUpperPct,
    tickLower,
    tickUpper,
    slippageTolerance,
    entrySwapSlippageTolerance,
    allowEntrySwap,
    confirmEntrySwap,
    walletId,
    ackLiquidityRisk,
    dcaEnabled,
    dcaPercentages,
    dcaIntervalSeconds,
    taskMode,
    rebalanceEnabled,
}) {
    const payload = {
        initData,
        chain,
        pool_address: poolAddress,
        pool_version: poolVersion,
        amount,
        range_input_mode: rangeInputMode || 'percentage',
        allow_entry_swap: Boolean(allowEntrySwap),
    };
    if ((rangeInputMode || 'percentage') === 'percentage') {
        payload.range_lower_pct = rangeLowerPct;
        payload.range_upper_pct = rangeUpperPct;
    }
    if (rangeInputMode === 'tick' || rangeInputMode === 'grid') {
        const lowerTick = Number(tickLower);
        const upperTick = Number(tickUpper);
        if (Number.isInteger(lowerTick)) payload.tick_lower = lowerTick;
        if (Number.isInteger(upperTick)) payload.tick_upper = upperTick;
    }
    const wid = Number(walletId);
    if (Number.isFinite(wid) && wid > 0) {
        payload.wallet_id = wid;
    }
    if (Number.isFinite(slippageTolerance)) {
        payload.slippage_tolerance = slippageTolerance;
    }
    if (Number.isFinite(entrySwapSlippageTolerance)) {
        payload.entry_swap_slippage_tolerance = entrySwapSlippageTolerance;
    }
    if (confirmEntrySwap) {
        payload.confirm_entry_swap = true;
    }
    if (ackLiquidityRisk) {
        payload.ack_liquidity_risk = true;
    }
    if (dcaEnabled !== undefined && dcaEnabled !== null) {
        payload.dca_enabled = Boolean(dcaEnabled);
    }
    if (Array.isArray(dcaPercentages) && dcaPercentages.length > 0) {
        payload.dca_percentages = dcaPercentages.map((v) => Number(v));
    }
    const dcaInterval = Number(dcaIntervalSeconds);
    if (Number.isFinite(dcaInterval) && dcaInterval >= 0) {
        payload.dca_interval_seconds = Math.round(dcaInterval * 1000) / 1000;
    }
    if (taskMode !== undefined && taskMode !== null && String(taskMode).trim()) {
        payload.task_mode = String(taskMode).trim();
    } else if (rebalanceEnabled !== undefined && rebalanceEnabled !== null) {
        payload.rebalance_enabled = Boolean(rebalanceEnabled);
    }
    return payload;
}

export async function previewOpenPosition({
    apiBaseUrl,
    initData,
    chain,
    poolAddress,
    poolVersion,
    amount,
    rangeInputMode,
    rangeLowerPct,
    rangeUpperPct,
    tickLower,
    tickUpper,
    slippageTolerance,
    entrySwapSlippageTolerance,
    allowEntrySwap,
    walletId,
    ackLiquidityRisk,
    dcaEnabled,
    dcaPercentages,
    dcaIntervalSeconds,
    taskMode,
    rebalanceEnabled,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const payload = buildOpenPositionPayload({
        initData,
        chain,
        poolAddress,
        poolVersion,
        amount,
        rangeInputMode,
        rangeLowerPct,
        rangeUpperPct,
        tickLower,
        tickUpper,
        slippageTolerance,
        entrySwapSlippageTolerance,
        allowEntrySwap,
        walletId,
        ackLiquidityRisk,
        dcaEnabled,
        dcaPercentages,
        dcaIntervalSeconds,
        taskMode,
        rebalanceEnabled,
    });
    const urls = [
        `${base}/api/open_position_preview`,
        `${base}/api/trading?endpoint=open_position_preview`,
    ];
    let lastError = null;
    for (let i = 0; i < urls.length; i += 1) {
        const resp = await fetch(urls[i], {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
            signal,
        });
        if (resp.ok) {
            return resp.json();
        }
        const detail = await readErrorDetails(resp);
        const rawMessage = String(detail.message || '').trim();
        const displayMessage = rawMessage === `HTTP ${resp.status}` || rawMessage === ''
            ? `获取前置兑换预览失败（HTTP ${resp.status}）`
            : rawMessage;
        const err = new Error(displayMessage);
        err.status = resp.status;
        if (detail.payload && typeof detail.payload === 'object') {
            err.payload = detail.payload;
            Object.assign(err, detail.payload);
        }
        lastError = err;
        const canFallback = i < urls.length - 1 && (
            rawMessage === `HTTP ${resp.status}` ||
            rawMessage === '' ||
            resp.status === 404 ||
            resp.status === 405
        );
        if (canFallback) {
            continue;
        }
        throw err;
    }
    throw lastError || new Error('获取前置兑换预览失败');
}

export async function prepareOpenPosition({
    apiBaseUrl,
    initData,
    chain,
    poolAddress,
    poolVersion,
    walletId,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const payload = {
        initData,
        chain,
        pool_address: poolAddress,
        pool_version: poolVersion,
    };
    const wid = Number(walletId);
    if (Number.isFinite(wid) && wid > 0) {
        payload.wallet_id = wid;
    }
    const urls = [
        `${base}/api/open_position_prepare`,
        `${base}/api/trading?endpoint=open_position_prepare`,
    ];
    let lastError = null;
    for (let i = 0; i < urls.length; i += 1) {
        const resp = await fetch(urls[i], {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
            signal,
        });
        if (resp.ok) {
            return resp.json();
        }
        const detail = await readErrorDetails(resp);
        const rawMessage = String(detail.message || '').trim();
        const displayMessage = rawMessage === `HTTP ${resp.status}` || rawMessage === ''
            ? `获取开仓预检测失败（HTTP ${resp.status}）`
            : rawMessage;
        const err = new Error(displayMessage);
        err.status = resp.status;
        if (detail.payload && typeof detail.payload === 'object') {
            err.payload = detail.payload;
            Object.assign(err, detail.payload);
        }
        lastError = err;
        const canFallback = i < urls.length - 1 && (
            rawMessage === `HTTP ${resp.status}` ||
            rawMessage === '' ||
            resp.status === 404 ||
            resp.status === 405
        );
        if (canFallback) {
            continue;
        }
        throw err;
    }
    throw lastError || new Error('获取开仓预检测失败');
}

export async function openPosition({
    apiBaseUrl,
    initData,
    chain,
    poolAddress,
    poolVersion,
    amount,
    rangeInputMode,
    rangeLowerPct,
    rangeUpperPct,
    tickLower,
    tickUpper,
    slippageTolerance,
    entrySwapSlippageTolerance,
    allowEntrySwap,
    confirmEntrySwap,
    walletId,
    ackLiquidityRisk,
    dcaEnabled,
    dcaPercentages,
    dcaIntervalSeconds,
    taskMode,
    rebalanceEnabled,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/trading?endpoint=open_position`;
    const payload = buildOpenPositionPayload({
        initData,
        chain,
        poolAddress,
        poolVersion,
        amount,
        rangeInputMode,
        rangeLowerPct,
        rangeUpperPct,
        tickLower,
        tickUpper,
        slippageTolerance,
        entrySwapSlippageTolerance,
        allowEntrySwap,
        confirmEntrySwap,
        walletId,
        ackLiquidityRisk,
        dcaEnabled,
        dcaPercentages,
        dcaIntervalSeconds,
        taskMode,
        rebalanceEnabled,
    });
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
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
    return resp.json();
}

export async function fetchSystemConfig({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=system_config`;
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

export async function updateSystemConfig({ apiBaseUrl, initData, config, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=system_config`;
    const payload = { initData, ...config };
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


// RPC Pool (Admin)

async function adminRPCPoolRequest({ apiBaseUrl, payload, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=rpc_pool`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload || {}),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminRPCPool({ apiBaseUrl, initData, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'list' },
        signal,
    });
}

export async function addAdminRPCEndpoint({ apiBaseUrl, initData, chain, transport, name, url, setCurrent, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: {
            initData,
            action: 'add',
            chain,
            transport,
            name,
            url,
            set_current: Boolean(setCurrent),
        },
        signal,
    });
}

export async function renameAdminRPCEndpoint({ apiBaseUrl, initData, endpointId, name, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'rename', endpoint_id: Number(endpointId), name },
        signal,
    });
}

export async function switchAdminRPCEndpoint({ apiBaseUrl, initData, endpointId, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'switch', endpoint_id: Number(endpointId) },
        signal,
    });
}

export async function disableAdminRPCEndpointNextMonth({ apiBaseUrl, initData, endpointId, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'disable', endpoint_id: Number(endpointId), disable_next_month: true },
        signal,
    });
}

export async function enableAdminRPCEndpoint({ apiBaseUrl, initData, endpointId, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'enable', endpoint_id: Number(endpointId) },
        signal,
    });
}

export async function deleteAdminRPCEndpoint({ apiBaseUrl, initData, endpointId, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'delete', endpoint_id: Number(endpointId) },
        signal,
    });
}

export async function checkAdminRPCEndpoint({ apiBaseUrl, initData, endpointId, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'check', endpoint_id: Number(endpointId) },
        signal,
    });
}

// Pool data sources (Admin)

async function adminPoolDataSourcesRequest({ apiBaseUrl, payload, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=pool_data_sources`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload || {}),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminPoolDataSources({ apiBaseUrl, initData, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'list' },
        signal,
    });
}

export async function addAdminPoolDataSource({
    apiBaseUrl,
    initData,
    name,
    sourceType,
    chain,
    timeframeMinutes,
    limit,
    baseUrl,
    pathTemplate,
    queryTemplate,
    protocols,
    dexes,
    setCurrent,
    signal,
}) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: {
            initData,
            action: 'add',
            name,
            source_type: sourceType,
            chain,
            timeframe_minutes: Number(timeframeMinutes),
            limit: Number(limit),
            base_url: baseUrl,
            path_template: pathTemplate,
            query_template: queryTemplate || {},
            protocols: Array.isArray(protocols) ? protocols : [],
            dexes: Array.isArray(dexes) ? dexes : [],
            set_current: Boolean(setCurrent),
        },
        signal,
    });
}

export async function switchAdminPoolDataSource({ apiBaseUrl, initData, sourceId, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'switch', source_id: Number(sourceId) },
        signal,
    });
}

export async function enableAdminPoolDataSource({ apiBaseUrl, initData, sourceId, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'enable', source_id: Number(sourceId) },
        signal,
    });
}

export async function disableAdminPoolDataSource({ apiBaseUrl, initData, sourceId, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'disable', source_id: Number(sourceId) },
        signal,
    });
}

export async function deleteAdminPoolDataSource({ apiBaseUrl, initData, sourceId, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'delete', source_id: Number(sourceId) },
        signal,
    });
}

export async function checkAdminPoolDataSource({ apiBaseUrl, initData, sourceId, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'check', source_id: Number(sourceId) },
        signal,
    });
}

async function adminPrivateZapRequest({ apiBaseUrl, payload, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=private_zap`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload || {}),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminPrivateZap({ apiBaseUrl, initData, signal }) {
    return adminPrivateZapRequest({
        apiBaseUrl,
        payload: { initData, action: 'list' },
        signal,
    });
}

export async function invalidateAdminPrivateZap({ apiBaseUrl, initData, chain, kind, signal }) {
    return adminPrivateZapRequest({
        apiBaseUrl,
        payload: { initData, action: 'invalidate', chain, kind },
        signal,
    });
}

export async function saveGlobalConfig({ apiBaseUrl, initData, config, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=global_config';
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, action: 'save', ...config }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || 'HTTP ' + resp.status);
    }
    return resp.json();
}

export async function fetchTradeHistory({ apiBaseUrl, initData, chain, status, limit, offset, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/positions?endpoint=trade_history';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (status) payload.status = String(status);
    if (Number.isFinite(limit)) payload.limit = limit;
    if (Number.isFinite(offset)) payload.offset = offset;
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

export async function walletSwapPreview({ apiBaseUrl, initData, chain, minValueUsd, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_preview';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (Number.isFinite(minValueUsd)) payload.min_value_usd = minValueUsd;
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

export async function walletSwapExecute({ apiBaseUrl, initData, chain, slippagePercent, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_execute';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (Number.isFinite(slippagePercent)) payload.slippage_percent = slippagePercent;
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

// --- Wallet CRUD ---
export async function walletCRUD({ apiBaseUrl, initData, action, privateKey, name, walletId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_crud';
    const payload = { initData, action };
    if (privateKey) payload.private_key = privateKey;
    if (name) payload.name = name;
    if (walletId) payload.wallet_id = Number(walletId);
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

// --- Single Token Swap ---
export async function walletSwapSingleQuote({ apiBaseUrl, initData, chain, walletId, fromToken, toToken, amount, slippagePercent, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_single';
    const payload = { initData, action: 'quote', from_token: fromToken, to_token: toToken, amount };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(slippagePercent)) payload.slippage_percent = slippagePercent;
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

export async function walletSwapSingleExecute({ apiBaseUrl, initData, chain, walletId, fromToken, toToken, amount, slippagePercent, provider, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_single';
    const payload = { initData, action: 'swap', from_token: fromToken, to_token: toToken, amount };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(slippagePercent)) payload.slippage_percent = slippagePercent;
    if (provider) payload.provider = String(provider);
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

export async function fetchPoolLiquidityDistribution({ apiBaseUrl, initData, chain, protocol, address, radius, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));
    if (protocol) params.set('protocol', String(protocol));
    if (address) params.set('address', String(address));
    if (Number.isFinite(radius)) params.set('radius', String(radius));
    const url = `${base}/api/pool_liquidity_distribution?${params.toString()}`;
    const resp = await fetch(url, { method: 'GET', signal });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}
