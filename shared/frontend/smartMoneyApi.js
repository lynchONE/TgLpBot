// Shared Smart Money API functions
const SM_BASE = '/api/sm';
const SM_UPLOAD_PROXY = '/api/sm_upload';

function normalizeBase(apiBaseUrl) {
    return String(apiBaseUrl || '').trim().replace(/\/$/, '');
}

function shouldProxyAvatarAsset(rawUrl) {
    const value = String(rawUrl || '').trim();
    if (!value || typeof window === 'undefined') return false;

    let parsed;
    try {
        parsed = new URL(value, window.location.origin);
    } catch {
        return false;
    }

    return window.location.protocol === 'https:' && parsed.protocol === 'http:';
}

export function resolveSMAvatarAssetUrl(avatarUrl) {
    const value = String(avatarUrl || '').trim();
    if (!value) return '';
    if (!shouldProxyAvatarAsset(value)) return value;

    const params = new URLSearchParams();
    params.set('endpoint', 'avatar_asset');
    params.set('url', value);
    return `/api/sm?${params.toString()}`;
}

function isSameOriginBase(base) {
    if (!base) return true;
    if (typeof window === 'undefined') return false;
    return base === window.location.origin;
}

function buildSMUrl(apiBaseUrl, endpoint, params) {
    const base = normalizeBase(apiBaseUrl);
    const search = params ? `?${params}` : '';
    if (isSameOriginBase(base)) {
        const proxyParams = new URLSearchParams(params || '');
        proxyParams.set('endpoint', endpoint);
        return `${base}/api/sm?${proxyParams.toString()}`;
    }
    return `${base}${SM_BASE}/${endpoint}${search}`;
}

function buildSMUploadUrl(apiBaseUrl, endpoint, params) {
    const base = normalizeBase(apiBaseUrl);
    const search = params ? `?${params}` : '';
    if (isSameOriginBase(base)) {
        const proxyParams = new URLSearchParams(params || '');
        proxyParams.set('endpoint', endpoint);
        return `${base}${SM_UPLOAD_PROXY}?${proxyParams.toString()}`;
    }
    return `${base}${SM_BASE}/${endpoint}${search}`;
}

function sanitizeErrorMessage(value, status, useStatusFallback = false) {
    const normalized = String(value || '').replace(/\s+/g, ' ').trim();
    if (!normalized) return status ? `HTTP ${status}` : '请求失败';
    if (/(?:^|\D)504(?:\D|$)|gateway time-?out/i.test(normalized)) {
        return '扫描服务超时（504 Gateway Timeout），请缩小时间范围后重试。';
    }
    if (/(?:^|\D)502(?:\D|$)|bad gateway/i.test(normalized)) {
        return '扫描服务暂时不可用（502 Bad Gateway），请稍后重试。';
    }
    if (/cloudflare|cdn-cgi|cf-error|<!doctype|<html/i.test(normalized)) {
        return `请求失败（HTTP ${status || '未知'}），服务返回了非 JSON 页面。`;
    }
    if (useStatusFallback && status === 504) return '扫描服务超时（504 Gateway Timeout），请缩小时间范围后重试。';
    if (useStatusFallback && status === 502) return '扫描服务暂时不可用（502 Bad Gateway），请稍后重试。';
    if (normalized.length > 240) return `${normalized.slice(0, 240)}...`;
    return normalized;
}

async function readErrorMessage(resp) {
    const text = await resp.text().catch(() => '');
    if (!text) return `HTTP ${resp.status}`;
    try {
        const json = JSON.parse(text);
        if (json?.message) return sanitizeErrorMessage(json.message, resp.status);
        if (json?.error) return sanitizeErrorMessage(json.error, resp.status);
    } catch {
        // ignore JSON parse errors and fall back to a sanitized text message
    }
    return sanitizeErrorMessage(text, resp.status, true);
}

async function readResponseJson(resp) {
    const text = await resp.text().catch(() => '');
    if (!text) throw new Error(`HTTP ${resp.status}: empty response`);
    try {
        return JSON.parse(text);
    } catch {
        throw new Error(sanitizeErrorMessage(text, resp.status, true));
    }
}

async function smRequest(url, options = {}) {
    const resp = await fetch(url, { cache: 'no-store', ...options });
    if (!resp.ok) {
        throw new Error(await readErrorMessage(resp));
    }
    const json = await readResponseJson(resp);
    if (json.code !== 0 && json.code !== undefined) {
        throw new Error(sanitizeErrorMessage(json.message || json.error || 'unknown error', resp.status));
    }
    return json.data;
}

async function goldenDogRequest(url, options = {}) {
    const resp = await fetch(url, { cache: 'no-store', ...options });
    if (!resp.ok) {
        throw new Error(await readErrorMessage(resp));
    }
    const json = await readResponseJson(resp);
    if (!json?.ok) {
        throw new Error(sanitizeErrorMessage(json?.message || json?.error || 'unknown error', resp.status));
    }
    return json;
}

export async function fetchSMWallets({ apiBaseUrl, page = 1, size = 10, keyword, source, active, signal }) {
    const params = new URLSearchParams();
    params.set('page', String(page));
    params.set('size', String(size));
    if (keyword) params.set('keyword', keyword);
    if (source) params.set('source', source);
    if (active !== undefined) params.set('active', String(active));
    return smRequest(buildSMUrl(apiBaseUrl, 'wallets', params.toString()), { signal });
}

export async function addSMWallet({ apiBaseUrl, address, label, chain, signal }) {
    const params = new URLSearchParams();
    if (chain) params.set('chain', String(chain));
    return smRequest(buildSMUrl(apiBaseUrl, 'wallets', params.toString()), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ address, label }),
        signal,
    });
}

export async function updateSMWallet({ apiBaseUrl, address, updates, chain, signal }) {
    const params = new URLSearchParams();
    params.set('address', String(address));
    if (chain) params.set('chain', String(chain));
    return smRequest(buildSMUrl(apiBaseUrl, 'wallets', params.toString()), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(updates),
        signal,
    });
}

export async function deleteSMWallet({ apiBaseUrl, address, chain, signal }) {
    const params = new URLSearchParams();
    params.set('address', String(address));
    if (chain) params.set('chain', String(chain));
    return smRequest(buildSMUrl(apiBaseUrl, 'wallets', params.toString()), {
        method: 'DELETE',
        signal,
    });
}

export async function fetchSMZombieWallets({ apiBaseUrl, days = 30, chain, signal }) {
    const params = new URLSearchParams();
    params.set('days', String(days));
    if (chain) params.set('chain', String(chain));
    return smRequest(buildSMUrl(apiBaseUrl, 'wallet_zombies', params.toString()), { signal });
}

export async function deleteSMZombieWallets({ apiBaseUrl, wallets, signal }) {
    return smRequest(buildSMUrl(apiBaseUrl, 'wallet_zombies', ''), {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ wallets }),
        signal,
    });
}

export async function fetchSMPoolLiquidityWalletCandidates({
    apiBaseUrl,
    chain = 'bsc',
    poolAddress,
    poolId,
    minAmountUsd,
    windowHours,
    startTime,
    endTime,
    limit,
    signal,
}) {
    const params = new URLSearchParams();
    if (chain) params.set('chain', String(chain));
    if (poolAddress) params.set('pool_address', String(poolAddress));
    if (poolId) params.set('pool_id', String(poolId));
    params.set('min_amount_usd', String(minAmountUsd));
    if (startTime) params.set('start_time', String(startTime));
    if (endTime) params.set('end_time', String(endTime));
    if (!startTime && !endTime && windowHours !== undefined) params.set('window_hours', String(windowHours));
    params.set('limit', String(limit));
    return smRequest(buildSMUrl(apiBaseUrl, 'pool_liquidity_wallet_candidates', params.toString()), { signal });
}

function buildPoolLiquidityCandidateParams({
    chain = 'bsc',
    poolAddress,
    poolId,
    minAmountUsd,
    windowHours,
    startTime,
    endTime,
    limit,
}) {
    const params = new URLSearchParams();
    if (chain) params.set('chain', String(chain));
    if (poolAddress) params.set('pool_address', String(poolAddress));
    if (poolId) params.set('pool_id', String(poolId));
    params.set('min_amount_usd', String(minAmountUsd));
    if (startTime) params.set('start_time', String(startTime));
    if (endTime) params.set('end_time', String(endTime));
    if (!startTime && !endTime && windowHours !== undefined) params.set('window_hours', String(windowHours));
    params.set('limit', String(limit));
    return params;
}

function parseSSEBlocks(buffer, onEvent) {
    let rest = buffer;
    for (;;) {
        const index = rest.indexOf('\n\n');
        if (index < 0) break;
        const block = rest.slice(0, index);
        rest = rest.slice(index + 2);
        let event = 'message';
        const data = [];
        block.split('\n').forEach((line) => {
            if (line.startsWith('event:')) event = line.slice(6).trim();
            if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
        });
        if (data.length > 0) {
            onEvent(event, data.join('\n'));
        }
    }
    return rest;
}

export async function streamSMPoolLiquidityWalletCandidates({
    apiBaseUrl,
    chain = 'bsc',
    poolAddress,
    poolId,
    minAmountUsd,
    windowHours,
    startTime,
    endTime,
    limit,
    signal,
    onStage,
    onCandidate,
    onWarning,
    onSummary,
    onDone,
    onError,
}) {
    if (typeof ReadableStream === 'undefined') {
        throw new Error('当前浏览器不支持流式扫描');
    }
    const params = buildPoolLiquidityCandidateParams({ chain, poolAddress, poolId, minAmountUsd, windowHours, startTime, endTime, limit });
    const resp = await fetch(buildSMUrl(apiBaseUrl, 'pool_liquidity_wallet_candidates_stream', params.toString()), {
        method: 'GET',
        signal,
    });
    if (!resp.ok) {
        throw new Error(await readErrorMessage(resp));
    }
    if (!resp.body) {
        throw new Error('流式扫描响应为空');
    }
    const decoder = new TextDecoder();
    const reader = resp.body.getReader();
    let buffer = '';
    const dispatch = (event, raw) => {
        let payload;
        try {
            payload = JSON.parse(raw);
        } catch {
            throw new Error('流式扫描事件格式错误');
        }
        if (event === 'stage') onStage?.(payload);
        else if (event === 'candidate') onCandidate?.(payload);
        else if (event === 'warning') onWarning?.(payload);
        else if (event === 'summary') onSummary?.(payload);
        else if (event === 'done') onDone?.(payload);
        else if (event === 'error') {
            onError?.(payload);
            throw new Error(sanitizeErrorMessage(payload?.message || '流式扫描失败'));
        }
    };
    try {
        for (;;) {
            const { value, done } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, '\n');
            buffer = parseSSEBlocks(buffer, dispatch);
        }
        buffer += decoder.decode().replace(/\r\n/g, '\n');
        if (buffer.trim()) parseSSEBlocks(`${buffer}\n\n`, dispatch);
    } finally {
        reader.releaseLock();
    }
}

export async function importSMPoolLiquidityWallets({
    apiBaseUrl,
    chain = 'bsc',
    poolAddress,
    poolId,
    wallets,
    labelPrefix,
    signal,
}) {
    return smRequest(buildSMUrl(apiBaseUrl, 'pool_liquidity_wallet_import', ''), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            chain,
            pool_address: poolAddress,
            pool_id: poolId,
            wallets,
            label_prefix: labelPrefix,
        }),
        signal,
    });
}

export const fetchSMTokenLiquidityWalletCandidates = fetchSMPoolLiquidityWalletCandidates;
export const importSMTokenLiquidityWallets = importSMPoolLiquidityWallets;

export async function uploadSMWalletAvatar({ apiBaseUrl, address, file, chain, signal }) {
    const params = new URLSearchParams();
    params.set('address', String(address));
    if (chain) params.set('chain', String(chain));
    const formData = new FormData();
    formData.set('avatar', file);
    return smRequest(buildSMUploadUrl(apiBaseUrl, 'wallet_avatar', params.toString()), {
        method: 'POST',
        body: formData,
        signal,
    });
}

export async function fetchSMContracts({ apiBaseUrl, signal }) {
    return smRequest(buildSMUrl(apiBaseUrl, 'contracts', ''), { signal });
}

export async function addSMContract({ apiBaseUrl, contract_address, description, signal }) {
    return smRequest(buildSMUrl(apiBaseUrl, 'contracts', ''), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ contract_address, description }),
        signal,
    });
}

export async function updateSMContract({ apiBaseUrl, address, updates, signal }) {
    const params = new URLSearchParams();
    params.set('address', String(address));
    return smRequest(buildSMUrl(apiBaseUrl, 'contracts', params.toString()), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(updates),
        signal,
    });
}

export async function deleteSMContract({ apiBaseUrl, address, signal }) {
    const params = new URLSearchParams();
    params.set('address', String(address));
    return smRequest(buildSMUrl(apiBaseUrl, 'contracts', params.toString()), {
        method: 'DELETE',
        signal,
    });
}

export async function fetchSMPools({ apiBaseUrl, page = 1, size = 10, keyword, protocol, source, minSmartMoneyUsd, maxFeeRate, minMarketCapUsd, signal }) {
    const params = new URLSearchParams();
    params.set('page', String(page));
    params.set('size', String(size));
    if (keyword) params.set('keyword', keyword);
    if (protocol) params.set('protocol', protocol);
    if (source) params.set('source', source);
    if (Number.isFinite(minSmartMoneyUsd)) params.set('min_smart_money_usd', String(minSmartMoneyUsd));
    if (Number.isFinite(maxFeeRate)) params.set('max_fee_rate', String(maxFeeRate));
    if (Number.isFinite(minMarketCapUsd)) params.set('min_fdv_usd', String(minMarketCapUsd));
    return smRequest(buildSMUrl(apiBaseUrl, 'pools', params.toString()), { signal });
}

export async function fetchSMPoolStats({ apiBaseUrl, poolAddress, signal }) {
    const params = new URLSearchParams();
    params.set('pool', String(poolAddress));
    return smRequest(buildSMUrl(apiBaseUrl, 'pools', params.toString()), { signal });
}

export async function fetchSMPoolFeeHeatmap({ apiBaseUrl, window = '1m', sort = 'rate', keyword, protocol, page = 1, size = 10, signal }) {
    const params = new URLSearchParams();
    params.set('window', String(window));
    params.set('sort', String(sort));
    params.set('page', String(page));
    params.set('size', String(size));
    if (keyword) params.set('keyword', keyword);
    if (protocol) params.set('protocol', protocol);
    return smRequest(buildSMUrl(apiBaseUrl, 'pool_fee_heatmap', params.toString()), { signal });
}

export async function fetchSMPositions({ apiBaseUrl, status = 'open', wallet, pool, protocol, page = 1, size = 20, orderBy, signal }) {
    const params = new URLSearchParams();
    params.set('status', status);
    params.set('page', String(page));
    params.set('size', String(size));
    if (wallet) params.set('wallet', wallet);
    if (pool) params.set('pool', pool);
    if (protocol) params.set('protocol', protocol);
    if (orderBy) params.set('order_by', orderBy);
    return smRequest(buildSMUrl(apiBaseUrl, 'positions', params.toString()), { signal });
}

export async function fetchSMPositionDetail({ apiBaseUrl, positionRef, positionId, signal }) {
    const params = new URLSearchParams();
    if (positionRef) params.set('position_ref', String(positionRef));
    if (positionId) params.set('position_id', String(positionId));
    return smRequest(buildSMUrl(apiBaseUrl, 'position_detail', params.toString()), { signal });
}

export async function fetchSMEvents({ apiBaseUrl, wallet, pool, page = 1, size = 20, signal }) {
    const params = new URLSearchParams();
    params.set('page', String(page));
    params.set('size', String(size));
    if (wallet) params.set('wallet', wallet);
    if (pool) params.set('pool', pool);
    return smRequest(buildSMUrl(apiBaseUrl, 'events', params.toString()), { signal });
}

export async function fetchSMStats({ apiBaseUrl, address, signal }) {
    const params = new URLSearchParams();
    if (address) params.set('address', String(address));
    return smRequest(buildSMUrl(apiBaseUrl, 'stats', params.toString()), { signal });
}

export async function fetchSMGoldenDogConfig({ apiBaseUrl, initData, chain = 'bsc', signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    if (initData) params.set('initData', initData);
    if (chain) params.set('chain', chain);
    return goldenDogRequest(`${base}/api/smart_money_golden_dog_config?${params.toString()}`, { signal });
}

export async function saveSMGoldenDogConfig({ apiBaseUrl, initData, chain = 'bsc', config, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return goldenDogRequest(`${base}/api/smart_money_golden_dog_config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, chain, ...(config || {}) }),
        signal,
    });
}

export async function testSMGoldenDogConfig({ apiBaseUrl, initData, chain = 'bsc', mode, intensity, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return goldenDogRequest(`${base}/api/smart_money_golden_dog_test`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, chain, mode, intensity }),
        signal,
    });
}

export async function fetchSMWatchWallets({ apiBaseUrl, initData, chain = 'bsc', signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    if (initData) params.set('initData', initData);
    if (chain) params.set('chain', chain);
    return goldenDogRequest(`${base}/api/smart_money_watch_wallets?${params.toString()}`, { signal });
}

export async function fetchSMWatchActivity({ apiBaseUrl, initData, chain = 'bsc', wallet, page = 1, size = 20, signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    if (initData) params.set('initData', initData);
    if (chain) params.set('chain', chain);
    if (wallet) params.set('wallet', wallet);
    params.set('page', String(page));
    params.set('size', String(size));
    return goldenDogRequest(`${base}/api/smart_money_watch_activity?${params.toString()}`, { signal });
}

export async function saveSMWatchWallets({ apiBaseUrl, initData, chain = 'bsc', walletAddress, watched, wallets, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return goldenDogRequest(`${base}/api/smart_money_watch_wallets`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            initData,
            chain,
            wallet_address: walletAddress,
            watched,
            wallets,
        }),
        signal,
    });
}

export async function fetchSMWatchOpenAlertConfig({ apiBaseUrl, initData, chain = 'bsc', signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    if (initData) params.set('initData', initData);
    if (chain) params.set('chain', chain);
    return goldenDogRequest(`${base}/api/smart_money_watch_open_alert_config?${params.toString()}`, { signal });
}

export async function saveSMWatchOpenAlertConfig({ apiBaseUrl, initData, chain = 'bsc', config, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return goldenDogRequest(`${base}/api/smart_money_watch_open_alert_config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, chain, ...(config || {}) }),
        signal,
    });
}

export async function testSMWatchOpenAlertConfig({ apiBaseUrl, initData, chain = 'bsc', signal }) {
    const base = normalizeBase(apiBaseUrl);
    return goldenDogRequest(`${base}/api/smart_money_watch_open_alert_test`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, chain }),
        signal,
    });
}

export async function fetchSMAutoFollow({ apiBaseUrl, initData, chain = 'bsc', signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    if (initData) params.set('initData', initData);
    if (chain) params.set('chain', chain);
    return goldenDogRequest(`${base}/api/smart_money_auto_follow?${params.toString()}`, { signal });
}

export async function saveSMAutoFollowConfig({ apiBaseUrl, initData, chain = 'bsc', config, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return goldenDogRequest(`${base}/api/smart_money_auto_follow`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, chain, action: 'save', config }),
        signal,
    });
}

export async function deleteSMAutoFollowConfig({ apiBaseUrl, initData, chain = 'bsc', id, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return goldenDogRequest(`${base}/api/smart_money_auto_follow`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, chain, action: 'delete', id }),
        signal,
    });
}

export function buildSMEventsWsUrl(apiBaseUrl) {
    const base = normalizeBase(apiBaseUrl) || (typeof window !== 'undefined' ? window.location.origin : '');
    if (!base) return '';
    try {
        const url = new URL(base, typeof window !== 'undefined' ? window.location.origin : 'http://localhost');
        url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
        url.pathname = '/ws/sm/events';
        url.search = '';
        url.hash = '';
        return url.toString();
    } catch {
        return '';
    }
}
