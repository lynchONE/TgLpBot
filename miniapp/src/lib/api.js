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

export async function fetchAutoMonitor({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=auto_monitor`;
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

export async function fetchAutoLPPnLCurve({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/positions?endpoint=autolp_pnl_curve`;
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

export async function setAutoLPGuardCompareToPeak({ apiBaseUrl, initData, guardCompareToPeak, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/settings?endpoint=autolp_config`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, guard_compare_to_peak: Boolean(guardCompareToPeak) }),
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

export async function fetchSmartMoneyOverview({
    apiBaseUrl,
    initData,
    chain,
    poolLimit,
    walletLimit,
    poolsWindowHours,
    pnlWindowHours,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));
    if (Number.isFinite(poolLimit)) params.set('pool_limit', String(poolLimit));
    if (Number.isFinite(walletLimit)) params.set('wallet_limit', String(walletLimit));
    if (Number.isFinite(poolsWindowHours)) params.set('pools_window_hours', String(poolsWindowHours));
    if (Number.isFinite(pnlWindowHours)) params.set('pnl_window_hours', String(pnlWindowHours));

    const qs = params.toString();
    const url = `${base}/api/smart_money${qs ? `?${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    const text = await resp.text().catch(() => '');
    if (!resp.ok) {
        throw new Error(text || `HTTP ${resp.status}`);
    }
    const body = String(text || '').trim();
    if (!body) {
        return {
            pools: [],
            wallets_24h: [],
            summary: {},
            warnings: [`smart_money 接口返回空响应体 (HTTP ${resp.status})`],
        };
    }
    try {
        return JSON.parse(body);
    } catch {
        throw new Error(`smart_money invalid JSON: ${body.slice(0, 120)}`);
    }
}

export async function fetchSmartMoneyPoolAdds({
    apiBaseUrl,
    initData,
    chain,
    poolVersion,
    poolId,
    windowHours,
    limit,
    feesLimit,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));
    if (poolVersion) params.set('pool_version', String(poolVersion));
    if (poolId) params.set('pool_id', String(poolId));
    if (Number.isFinite(windowHours)) params.set('window_hours', String(windowHours));
    if (Number.isFinite(limit)) params.set('limit', String(limit));
    if (Number.isFinite(feesLimit)) params.set('fees_limit', String(feesLimit));

    const qs = params.toString();
    const url = `${base}/api/smart_money_pool_adds${qs ? `?${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    const text = await resp.text().catch(() => '');
    if (!resp.ok) {
        throw new Error(text || `HTTP ${resp.status}`);
    }
    const body = String(text || '').trim();
    if (!body) {
        return {
            chain: String(chain || 'bsc'),
            window_sec: 0,
            pool: {
                pool_version: String(poolVersion || ''),
                pool_id: String(poolId || ''),
            },
            wallets: [],
            warnings: [`smart_money_pool_adds 鎺ュ彛杩斿洖绌哄搷搴斾綋 (HTTP ${resp.status})`],
        };
    }
    try {
        return JSON.parse(body);
    } catch {
        throw new Error(`smart_money_pool_adds invalid JSON: ${body.slice(0, 120)}`);
    }
}

export async function fetchSmartMoneyWalletPositions({
    apiBaseUrl,
    initData,
    chain,
    walletAddress,
    windowHours,
    limit,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));
    if (walletAddress) params.set('wallet_address', String(walletAddress));
    if (Number.isFinite(windowHours)) params.set('window_hours', String(windowHours));
    if (Number.isFinite(limit)) params.set('limit', String(limit));

    const qs = params.toString();
    const url = `${base}/api/smart_money_wallet_positions${qs ? `?${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    const text = await resp.text().catch(() => '');
    if (!resp.ok) {
        throw new Error(text || `HTTP ${resp.status}`);
    }
    const body = String(text || '').trim();
    if (!body) {
        return {
            chain: String(chain || 'bsc'),
            wallet_address: String(walletAddress || ''),
            positions: [],
            warnings: [`smart_money_wallet_positions 接口返回空响应体 (HTTP ${resp.status})`],
        };
    }
    try {
        return JSON.parse(body);
    } catch {
        throw new Error(`smart_money_wallet_positions invalid JSON: ${body.slice(0, 120)}`);
    }
}

export async function fetchSmartMoneyFollowConfig({
    apiBaseUrl,
    initData,
    chain,
    walletAddress,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));
    if (walletAddress) params.set('wallet_address', String(walletAddress));

    const qs = params.toString();
    const url = `${base}/api/smart_money_follow_config${qs ? `?${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    const text = await resp.text().catch(() => '');
    if (!resp.ok) {
        throw new Error(text || `HTTP ${resp.status}`);
    }
    const body = String(text || '').trim();
    if (!body) {
        return {
            config: {
                chain: String(chain || 'bsc'),
                wallet_address: String(walletAddress || ''),
                enabled: false,
                max_total_amount_usdt: 0,
                per_trade_amount_usdt: 0,
                delay_min_seconds: 0,
                delay_max_seconds: 60,
            },
            warnings: [`smart_money_follow_config 接口返回空响应体 (HTTP ${resp.status})`],
        };
    }
    try {
        return JSON.parse(body);
    } catch {
        throw new Error(`smart_money_follow_config invalid JSON: ${body.slice(0, 120)}`);
    }
}

export async function fetchSmartMoneyFollowConfigs({
    apiBaseUrl,
    initData,
    chain,
    enabledOnly = true,
    limit = 100,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));
    params.set('enabled_only', enabledOnly ? '1' : '0');
    if (Number.isFinite(limit)) params.set('limit', String(limit));

    const qs = params.toString();
    const url = `${base}/api/smart_money_follow_configs${qs ? `?${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    const text = await resp.text().catch(() => '');
    if (!resp.ok) {
        throw new Error(text || `HTTP ${resp.status}`);
    }
    const body = String(text || '').trim();
    if (!body) {
        return {
            chain: String(chain || 'bsc'),
            configs: [],
            enabled_count: 0,
            warnings: [`smart_money_follow_configs 接口返回空响应体 (HTTP ${resp.status})`],
        };
    }
    try {
        return JSON.parse(body);
    } catch {
        throw new Error(`smart_money_follow_configs invalid JSON: ${body.slice(0, 120)}`);
    }
}

export async function saveSmartMoneyFollowConfig({
    apiBaseUrl,
    initData,
    chain,
    walletAddress,
    enabled,
    maxTotalAmountUSDT,
    perTradeAmountUSDT,
    delayMinSeconds,
    delayMaxSeconds,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/smart_money_follow_config`;

    const normalizedEnabled = (() => {
        if (typeof enabled === 'boolean') return enabled;
        if (typeof enabled === 'number') return enabled !== 0;
        const text = String(enabled ?? '').trim().toLowerCase();
        if (!text) return false;
        return text === '1' || text === 'true' || text === 'yes' || text === 'on';
    })();

    const payload = {
        initData,
        chain: chain ? String(chain) : undefined,
        wallet_address: walletAddress ? String(walletAddress) : undefined,
        enabled: normalizedEnabled,
        max_total_amount_usdt: Number.isFinite(Number(maxTotalAmountUSDT)) ? Number(maxTotalAmountUSDT) : undefined,
        per_trade_amount_usdt: Number.isFinite(Number(perTradeAmountUSDT)) ? Number(perTradeAmountUSDT) : undefined,
        delay_min_seconds: Number.isFinite(Number(delayMinSeconds)) ? Number(delayMinSeconds) : undefined,
        delay_max_seconds: Number.isFinite(Number(delayMaxSeconds)) ? Number(delayMaxSeconds) : undefined,
    };

    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    const text = await resp.text().catch(() => '');
    if (!resp.ok) {
        throw new Error(text || `HTTP ${resp.status}`);
    }
    const body = String(text || '').trim();
    if (!body) {
        return { ok: true };
    }
    try {
        return JSON.parse(body);
    } catch {
        throw new Error(`smart_money_follow_config invalid JSON: ${body.slice(0, 120)}`);
    }
}

export async function fetchSmartMoneyGoldenDogConfig({ apiBaseUrl, initData, chain, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));

    const qs = params.toString();
    const url = `${base}/api/smart_money_golden_dog_config${qs ? `?${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    const text = await resp.text().catch(() => '');
    if (!resp.ok) {
        throw new Error(text || `HTTP ${resp.status}`);
    }
    const body = String(text || '').trim();
    if (!body) {
        return {
            config: {
                chain: String(chain || 'bsc'),
                enabled: false,
                min_wallets: 3,
                window_minutes: 10,
                cooldown_minutes: 30,
            },
            warnings: [`smart_money_golden_dog_config 接口返回空响应体 (HTTP ${resp.status})`],
        };
    }
    try {
        return JSON.parse(body);
    } catch {
        throw new Error(`smart_money_golden_dog_config invalid JSON: ${body.slice(0, 120)}`);
    }
}

export async function saveSmartMoneyGoldenDogConfig({
    apiBaseUrl,
    initData,
    chain,
    enabled,
    minWallets,
    windowMinutes,
    cooldownMinutes,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/smart_money_golden_dog_config`;

    const payload = {
        initData,
        chain: chain ? String(chain) : undefined,
        enabled: typeof enabled === 'boolean' ? enabled : Boolean(enabled),
        min_wallets: Number.isFinite(Number(minWallets)) ? Number(minWallets) : undefined,
        window_minutes: Number.isFinite(Number(windowMinutes)) ? Number(windowMinutes) : undefined,
        cooldown_minutes: Number.isFinite(Number(cooldownMinutes)) ? Number(cooldownMinutes) : undefined,
    };

    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    const text = await resp.text().catch(() => '');
    if (!resp.ok) {
        throw new Error(text || `HTTP ${resp.status}`);
    }
    const body = String(text || '').trim();
    if (!body) {
        return { ok: true };
    }
    try {
        return JSON.parse(body);
    } catch {
        throw new Error(`smart_money_golden_dog_config invalid JSON: ${body.slice(0, 120)}`);
    }
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

export async function fetchAdminAutoLPStats({ apiBaseUrl, initData, userId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=autolp_stats`;
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
    const url = `${base}/api/admin?endpoint=autolp_disable`;
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

export async function setAdminSmartMoneyEnabled({ apiBaseUrl, initData, userId, enabled, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=user_access`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, userId, smartMoneyEnabled: enabled }),
        signal,
    });
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
    const url = `${base}/api/pools?endpoint=hot_pools${qs ? `&${qs}` : ''}`;

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

export async function openPosition({ apiBaseUrl, initData, chain, poolAddress, poolVersion, amount, rangeLowerPct, rangeUpperPct, slippageTolerance, allowEntrySwap, walletId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/trading?endpoint=open_position`;
    const payload = {
        initData,
        chain,
        pool_address: poolAddress,
        pool_version: poolVersion,
        amount,
        range_lower_pct: rangeLowerPct,
        range_upper_pct: rangeUpperPct,
        allow_entry_swap: Boolean(allowEntrySwap),
    };
    const wid = Number(walletId);
    if (Number.isFinite(wid) && wid > 0) {
        payload.wallet_id = wid;
    }
    if (Number.isFinite(slippageTolerance)) {
        payload.slippage_tolerance = slippageTolerance;
    }
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

// 黑名单 API

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

export async function fetchBlacklist({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/trading?endpoint=blacklist&initData=${encodeURIComponent(initData)}`;
    const resp = await fetch(url, { method: 'GET', signal });
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

export async function addToBlacklist({ apiBaseUrl, initData, poolAddress, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/trading?endpoint=blacklist`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, pool_address: poolAddress, action: 'add' }),
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

export async function removeFromBlacklist({ apiBaseUrl, initData, poolAddress, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/trading?endpoint=blacklist`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, pool_address: poolAddress, action: 'remove' }),
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

// 冷却 API

export async function fetchCooldowns({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/trading?endpoint=cooldowns&initData=${encodeURIComponent(initData)}`;
    const resp = await fetch(url, { method: 'GET', signal });
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

export async function removeCooldown({ apiBaseUrl, initData, tradingPair, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/trading?endpoint=cooldowns&initData=${encodeURIComponent(initData)}`;
    const resp = await fetch(url, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ trading_pair: tradingPair }),
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

export async function saveGlobalConfig({ apiBaseUrl, initData, config, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=global_config';
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, ...config }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || 'HTTP ' + resp.status);
    }
    return resp.json();
}

// ─── Smart Money Watched Wallets ────────────────────────────────────

export async function fetchSmartMoneyWatchedWallets({ apiBaseUrl, initData, chain, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));
    const qs = params.toString();
    const url = `${base}/api/smart_money_watched_wallets${qs ? `?${qs}` : ''}`;
    const resp = await fetch(url, { method: 'GET', signal });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function addSmartMoneyWatchedWallets({ apiBaseUrl, initData, chain, wallets, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/smart_money_watched_wallets`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, chain, wallets }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function removeSmartMoneyWatchedWallets({ apiBaseUrl, initData, chain, walletAddresses, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/smart_money_watched_wallets`;
    const resp = await fetch(url, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, chain, wallet_addresses: walletAddresses }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function updateSmartMoneyWatchedWalletLabel({ apiBaseUrl, initData, chain, walletAddress, label, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/smart_money_watched_wallets`;
    const resp = await fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, chain, wallet_address: walletAddress, label }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

// ─── Smart Money 24h Pool Adds ──────────────────────────────────────

export async function fetchSmartMoney24hPoolAdds({ apiBaseUrl, initData, chain, windowHours, poolLimit, topWalletLimit, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));
    if (Number.isFinite(windowHours)) params.set('window_hours', String(windowHours));
    if (Number.isFinite(poolLimit)) params.set('pool_limit', String(poolLimit));
    if (Number.isFinite(topWalletLimit)) params.set('top_wallet_limit', String(topWalletLimit));
    const qs = params.toString();
    const url = `${base}/api/smart_money_24h_pool_adds${qs ? `?${qs}` : ''}`;
    const resp = await fetch(url, { method: 'GET', signal });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}
