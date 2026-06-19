import { requestJson, timeoutSignal } from './api/request';
export {
    openPosition,
    prepareOpenPosition,
    previewOpenPosition,
} from './api/openPosition';
export {
    addAdminOKXConfig,
    addAdminPoolDataSource,
    addAdminRPCEndpoint,
    checkAdminOKXConfig,
    checkAdminPoolDataSource,
    checkAdminRPCEndpoint,
    deleteAdminOKXConfig,
    deleteAdminPoolDataSource,
    deleteAdminRPCEndpoint,
    disableAdminOKXConfig,
    disableAdminOKXConfigNextMonth,
    disableAdminPoolDataSource,
    disableAdminRPCEndpointNextMonth,
    enableAdminOKXConfig,
    enableAdminPoolDataSource,
    enableAdminRPCEndpoint,
    fetchAdminOKXPool,
    fetchAdminPoolDataSources,
    fetchAdminPrivateZap,
    fetchAdminRPCPool,
    fetchSystemConfig,
    invalidateAdminPrivateZap,
    renameAdminOKXConfig,
    renameAdminRPCEndpoint,
    saveGlobalConfig,
    switchAdminOKXConfig,
    switchAdminPoolDataSource,
    switchAdminRPCEndpoint,
    updateAdminOKXConfig,
    updateAdminPoolDataSource,
    updateSystemConfig,
} from './api/adminConfig';
export {
    clearAssetLPPnLAdjustment,
    clearAssetLPPnLBaseline,
    fetchAdminSmartMoneyLeaderboard,
    fetchAdminSmartMoneyOverview,
    fetchAdminSmartMoneyWallet,
    fetchAssetHistory,
    fetchAssetLPStats,
    fetchAssetOverview,
    saveAssetLPPnLAdjustment,
    saveAssetLPPnLBaseline,
} from './api/assets';
export {
    cancelWalletSwapLimitOrder,
    createWalletSwapLimitOrder,
    fetchPoolLiquidityDistribution,
    fetchTradeHistory,
    fetchWalletSwapHistory,
    fetchWalletSwapLimitOrders,
    fetchWalletSwapTokenMetadata,
    walletCRUD,
    walletSwapExecute,
    walletSwapPreview,
    walletSwapSingleExecute,
    walletSwapSingleQuote,
} from './api/walletSwap';

async function adminWorkbenchRequest({ apiBaseUrl, endpoint, payload, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=${endpoint}`;
    return requestJson(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload || {}),
        signal,
    });
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
    const url = `${base}/api/me`;
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

export async function stopTask({ apiBaseUrl, initData, taskId, exitPercent, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=stop`;
    const payload = { initData, taskId };
    if (exitPercent !== undefined && exitPercent !== null) payload.exitPercent = Number(exitPercent);
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

export async function withdrawLiquidity({ apiBaseUrl, initData, taskId, exitPercent, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=withdraw_liquidity`;
    const payload = { initData, taskId };
    if (exitPercent !== undefined && exitPercent !== null) payload.exitPercent = Number(exitPercent);
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

export async function addLiquidity({ apiBaseUrl, initData, taskId, amountUsdt, slippageTolerance, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/task_action?action=add_liquidity`;
    const body = { initData, taskId, amountUsdt: Number(amountUsdt) };
    if (Number.isFinite(Number(slippageTolerance))) {
        body.slippageTolerance = Number(slippageTolerance);
    }
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
    const timeout = timeoutSignal(signal, 8000, 'wallets request timeout');
    let resp;
    try {
        resp = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
            signal: timeout.signal,
        });
    } catch (err) {
        if (timeout.signal.aborted && !signal?.aborted) {
            throw new Error('钱包列表请求超时');
        }
        throw err;
    } finally {
        timeout.clear();
    }
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
    const data = await resp.json();
    if (!data || !Array.isArray(data.wallets)) {
        throw new Error('钱包列表响应格式错误');
    }
    return data;
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
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_access',
        payload: { initData, action: 'get', user_id: Number(userId) },
        signal,
    });
}

export async function fetchAdminAccessList({ apiBaseUrl, initData, page = 1, pageSize = 20, query = '', signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_access',
        payload: { initData, action: 'list', page, page_size: pageSize, query },
        signal,
    });
}

export async function updateAdminUserAccess({ apiBaseUrl, initData, userId, patch, signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_access',
        payload: { initData, action: 'update', user_id: Number(userId), ...(patch || {}) },
        signal,
    });
}

export async function revokeAdminUserAccess({ apiBaseUrl, initData, userId, signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_access',
        payload: { initData, action: 'revoke', user_id: Number(userId) },
        signal,
    });
}

export async function restoreAdminUserAccess({ apiBaseUrl, initData, userId, signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_access',
        payload: { initData, action: 'restore', user_id: Number(userId) },
        signal,
    });
}

export async function fetchAdminAuthCodes({ apiBaseUrl, initData, page = 1, pageSize = 20, query = '', signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_auth_codes',
        payload: { initData, action: 'list', page, page_size: pageSize, query },
        signal,
    });
}

export async function createAdminAuthCode({ apiBaseUrl, initData, payload, signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_auth_codes',
        payload: { initData, action: 'create', ...(payload || {}) },
        signal,
    });
}

export async function updateAdminAuthCode({ apiBaseUrl, initData, codeId, patch, signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_auth_codes',
        payload: { initData, action: 'update', code_id: Number(codeId), ...(patch || {}) },
        signal,
    });
}

export async function disableAdminAuthCode({ apiBaseUrl, initData, codeId, signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_auth_codes',
        payload: { initData, action: 'disable', code_id: Number(codeId) },
        signal,
    });
}

export async function enableAdminAuthCode({ apiBaseUrl, initData, codeId, signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_auth_codes',
        payload: { initData, action: 'enable', code_id: Number(codeId) },
        signal,
    });
}

export async function fetchAdminAnnouncements({ apiBaseUrl, initData, page = 1, pageSize = 20, signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_announcements',
        payload: { initData, action: 'list', page, page_size: pageSize },
        signal,
    });
}

export async function publishAdminAnnouncement({ apiBaseUrl, initData, title, content, signal }) {
    return adminWorkbenchRequest({
        apiBaseUrl,
        endpoint: 'admin_announcements',
        payload: { initData, action: 'publish', title, content },
        signal,
    });
}

export async function fetchHotPools({ apiBaseUrl, initData, sort, chain, timeframeMinutes, limit, dex, includePools, maxFeeRate, minMarketCapUsd, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (sort) params.set('sort', String(sort));
    if (chain) params.set('chain', String(chain));
    if (Number.isFinite(timeframeMinutes)) params.set('timeframe_minutes', String(timeframeMinutes));
    if (Number.isFinite(limit)) params.set('limit', String(limit));
    if (dex) params.set('dex', String(dex));
    if (Number.isFinite(maxFeeRate)) params.set('max_fee_rate', String(maxFeeRate));
    if (Number.isFinite(minMarketCapUsd)) params.set('min_fdv_usd', String(minMarketCapUsd));
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
