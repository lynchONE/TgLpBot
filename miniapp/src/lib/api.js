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

export async function fetchHotPools({ apiBaseUrl, sort, chain, timeframeMinutes, limit, dex, includePools, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
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
    const url = `${base}/api/pools?endpoint=pool_ohlcv${qs ? `&${qs}` : ''}`;

    const resp = await fetch(url, { method: 'GET', signal });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function openPosition({ apiBaseUrl, initData, poolAddress, poolVersion, amount, rangeLowerPct, rangeUpperPct, allowEntrySwap, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/trading?endpoint=open_position`;
    const payload = {
        initData,
        pool_address: poolAddress,
        pool_version: poolVersion,
        amount,
        range_lower_pct: rangeLowerPct,
        range_upper_pct: rangeUpperPct,
        allow_entry_swap: Boolean(allowEntrySwap),
    };
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
