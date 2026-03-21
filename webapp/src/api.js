function normalizeBaseUrl(apiBaseUrl) {
  return String(apiBaseUrl || '').trim().replace(/\/$/, '');
}

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

async function readErrorMessage(resp) {
  const text = await resp.text().catch(() => '');
  if (!text) return `HTTP ${resp.status}`;
  try {
    const parsed = JSON.parse(text);
    if (parsed?.message) return String(parsed.message);
  } catch {
    // ignore JSON parse errors
  }
  return text;
}

async function requestJson(url, options) {
  const resp = await fetch(url, options);
  if (!resp.ok) {
    const err = new Error(await readErrorMessage(resp));
    err.status = resp.status;
    throw err;
  }
  return resp.json();
}

export async function fetchHotPools({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  sort = 'fees',
  timeframeMinutes = 5,
  limit = 50,
  tokenAddress,
  includePools,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const params = new URLSearchParams();
  if (initData) params.set('initData', String(initData));
  if (chain) params.set('chain', String(chain));
  if (sort) params.set('sort', String(sort));
  if (Number.isFinite(timeframeMinutes))
    params.set('timeframe_minutes', String(timeframeMinutes));
  if (Number.isFinite(limit)) params.set('limit', String(limit));
  if (tokenAddress) params.set('token_address', String(tokenAddress));
  if (Array.isArray(includePools) && includePools.length > 0) {
    params.set('include_pools', includePools.join(','));
  }

  const qs = params.toString();
  const url = `${base}/api/pools${qs ? `?${qs}` : ''}`;
  return requestJson(url, { method: 'GET', signal });
}

export async function fetchTokenCandles({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  tokenAddress,
  bar = '1m',
  limit = 240,
  before,
  after,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const params = new URLSearchParams();
  if (initData) params.set('initData', String(initData));
  if (chain) params.set('chain', String(chain));
  if (tokenAddress) params.set('token_address', String(tokenAddress));
  if (bar) params.set('bar', String(bar));
  if (Number.isFinite(limit)) params.set('limit', String(limit));
  if (before !== undefined && before !== null && String(before).trim()) params.set('before', String(before).trim());
  if (after !== undefined && after !== null && String(after).trim()) params.set('after', String(after).trim());

  const qs = params.toString();
  const url = `${base}/api/token_candles${qs ? `?${qs}` : ''}`;
  return requestJson(url, { method: 'GET', signal });
}

export async function fetchSmartMoneyPoolMarkers({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  poolId,
  poolVersion,
  bucketSec = 300,
  windowHours = 12,
  limit = 300,
  startTs,
  endTs,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const params = new URLSearchParams();
  if (initData) params.set('initData', String(initData));
  if (chain) params.set('chain', String(chain));
  if (poolId) params.set('pool_id', String(poolId));
  if (poolVersion) params.set('pool_version', String(poolVersion));
  if (Number.isFinite(bucketSec)) params.set('bucket_sec', String(bucketSec));
  if (Number.isFinite(windowHours)) params.set('window_hours', String(windowHours));
  if (Number.isFinite(limit)) params.set('limit', String(limit));
  if (Number.isFinite(startTs) && startTs > 0) params.set('start_ts', String(Math.floor(startTs)));
  if (Number.isFinite(endTs) && endTs > 0) params.set('end_ts', String(Math.floor(endTs)));

  const qs = params.toString();
  const url = `${base}/api/smart_money_pool_markers${qs ? `?${qs}` : ''}`;
  return requestJson(url, { method: 'GET', signal });
}

export async function fetchRealtimePositions({ apiBaseUrl, initData, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/positions?endpoint=realtime_positions`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData }),
    signal,
  });
}

export async function fetchAssetOverview({ apiBaseUrl, initData, forceRefresh = false, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/positions?endpoint=assets_overview`;
  const cacheKey = `asset-overview:${base}:${initData}`;
  return resolveAssetCachedPayload({
    cacheKey,
    forceRefresh,
    load: async () => {
      const payload = await requestJson(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, force_refresh: forceRefresh }),
        signal,
      });
      return payload?.data ?? payload;
    },
  });
}

export async function fetchAssetHistory({ apiBaseUrl, initData, days = 30, forceRefresh = false, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/positions?endpoint=assets_history`;
  const cacheKey = `asset-history:${base}:${initData}:${days}`;
  return resolveAssetCachedPayload({
    cacheKey,
    forceRefresh,
    load: async () => {
      const payload = await requestJson(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, days, force_refresh: forceRefresh }),
        signal,
      });
      return payload?.data ?? payload;
    },
  });
}

export async function fetchAssetLPStats({ apiBaseUrl, initData, forceRefresh = false, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/positions?endpoint=assets_lp_stats`;
  const cacheKey = `asset-lp:${base}:${initData}`;
  return resolveAssetCachedPayload({
    cacheKey,
    forceRefresh,
    load: async () => {
      const payload = await requestJson(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, force_refresh: forceRefresh }),
        signal,
      });
      return payload?.data ?? payload;
    },
  });
}

export async function fetchPositionProfitPoster({ apiBaseUrl, initData, taskId, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/position_profit_poster`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData, taskId }),
    signal,
  });
}

export async function generateLoginCode({ apiBaseUrl, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/web_login?endpoint=generate_code`;
  return requestJson(url, { method: 'POST', signal });
}

export async function checkLoginCode({ apiBaseUrl, code, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/web_login?endpoint=check_code`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
    signal,
  });
}

export async function fetchWallets({ apiBaseUrl, initData, chain, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/settings?endpoint=wallets`;
  const payload = { initData };
  if (chain) payload.chain = String(chain);
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  });
}

export async function fetchAdminSmartMoneyOverview({ apiBaseUrl, initData, days = 7, forceRefresh = false, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/admin?endpoint=assets_smart_money_overview`;
  const cacheKey = `admin-smart-money-overview:${base}:${initData}:${days}`;
  return resolveAssetCachedPayload({
    cacheKey,
    forceRefresh,
    load: async () => {
      const payload = await requestJson(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, days, force_refresh: forceRefresh }),
        signal,
      });
      return payload?.data ?? payload;
    },
  });
}

export async function fetchAdminSmartMoneyWallet({
  apiBaseUrl,
  initData,
  address,
  chainId,
  days = 7,
  forceRefresh = false,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/admin?endpoint=assets_smart_money_wallet`;
  const cacheKey = `admin-smart-money-wallet:${base}:${initData}:${String(address || '').toLowerCase()}:${chainId}:${days}`;
  return resolveAssetCachedPayload({
    cacheKey,
    forceRefresh,
    load: async () => {
      const payload = await requestJson(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, address, chain_id: chainId, days, force_refresh: forceRefresh }),
        signal,
      });
      return payload?.data ?? payload;
    },
  });
}

export async function fetchAdminSmartMoneyLeaderboard({
  apiBaseUrl,
  initData,
  days = 1,
  metric = 'pnl',
  limit = 20,
  forceRefresh = false,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/admin?endpoint=assets_smart_money_leaderboard`;
  const cacheKey = `admin-smart-money-leaderboard:${base}:${initData}:${days}:${metric}:${limit}`;
  return resolveAssetCachedPayload({
    cacheKey,
    forceRefresh,
    load: async () => {
      const payload = await requestJson(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, days, metric, limit, force_refresh: forceRefresh }),
        signal,
      });
      return payload?.data ?? payload;
    },
  });
}

export async function fetchAdminOnlineUsers({ apiBaseUrl, initData, limit, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/admin?endpoint=online_users`;
  const payload = { initData };
  if (Number.isFinite(limit)) payload.limit = limit;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  });
}

export async function fetchAdminActiveTasks({ apiBaseUrl, initData, limit, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/admin?endpoint=active_tasks`;
  const payload = { initData };
  if (Number.isFinite(limit)) payload.limit = limit;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  });
}

export async function fetchAdminRealtimePositions({ apiBaseUrl, initData, userId, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/admin?endpoint=realtime_positions`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData, userId }),
    signal,
  });
}

export async function fetchSystemConfig({ apiBaseUrl, initData, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/admin?endpoint=system_config`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData }),
    signal,
  });
}

export async function updateSystemConfig({ apiBaseUrl, initData, config, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/admin?endpoint=system_config`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData, ...(config || {}) }),
    signal,
  });
}

async function adminRPCPoolRequest({ apiBaseUrl, payload, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/admin?endpoint=rpc_pool`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload || {}),
    signal,
  });
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

async function adminPrivateZapRequest({ apiBaseUrl, payload, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/admin?endpoint=private_zap`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload || {}),
    signal,
  });
}

export async function fetchAdminPrivateZap({ apiBaseUrl, initData, signal }) {
  return adminPrivateZapRequest({
    apiBaseUrl,
    payload: { initData, action: 'list' },
    signal,
  });
}

export async function invalidateAdminPrivateZap({ apiBaseUrl, initData, chain, signal }) {
  return adminPrivateZapRequest({
    apiBaseUrl,
    payload: { initData, action: 'invalidate', chain },
    signal,
  });
}

export async function openPosition({
  apiBaseUrl,
  initData,
  chain,
  poolAddress,
  poolVersion,
  amount,
  rangeLowerPct,
  rangeUpperPct,
  slippageTolerance,
  allowEntrySwap,
  walletId,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
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
  if (Number.isFinite(slippageTolerance)) payload.slippage_tolerance = slippageTolerance;
  const wid = Number(walletId);
  if (Number.isFinite(wid) && wid > 0) payload.wallet_id = wid;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  });
}

export async function previewCreatePool({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  protocol,
  walletId,
  tokenAAddress,
  tokenBAddress,
  feeTier,
  tickSpacing,
  initialPrice,
  mode = 'create_and_seed',
  rangeMode = 'full_range',
  amountMode = 'dual_exact',
  minPrice,
  maxPrice,
  amountA,
  amountB,
  slippageTolerance,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/trading?endpoint=create_pool_preview`;
  const payload = {
    initData,
    chain,
    protocol,
    token_a_address: tokenAAddress,
    token_b_address: tokenBAddress,
    fee_tier: feeTier,
    mode,
    range_mode: rangeMode,
    amount_mode: amountMode,
  };
  const wid = Number(walletId);
  if (Number.isFinite(wid) && wid > 0) payload.wallet_id = wid;
  const spacing = Number(tickSpacing);
  if (Number.isFinite(spacing) && spacing > 0) payload.tick_spacing = spacing;
  if (initialPrice !== undefined && initialPrice !== null && String(initialPrice).trim()) {
    payload.initial_price = String(initialPrice).trim();
  }
  if (minPrice !== undefined && minPrice !== null && String(minPrice).trim()) {
    payload.min_price = String(minPrice).trim();
  }
  if (maxPrice !== undefined && maxPrice !== null && String(maxPrice).trim()) {
    payload.max_price = String(maxPrice).trim();
  }
  if (amountA !== undefined && amountA !== null && String(amountA).trim()) {
    payload.amount_a = String(amountA).trim();
  }
  if (amountB !== undefined && amountB !== null && String(amountB).trim()) {
    payload.amount_b = String(amountB).trim();
  }
  if (Number.isFinite(slippageTolerance)) payload.slippage_tolerance = slippageTolerance;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  });
}

export async function executeCreatePool({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  protocol,
  walletId,
  tokenAAddress,
  tokenBAddress,
  feeTier,
  tickSpacing,
  initialPrice,
  mode = 'create_and_seed',
  rangeMode = 'full_range',
  amountMode = 'dual_exact',
  minPrice,
  maxPrice,
  amountA,
  amountB,
  slippageTolerance,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/trading?endpoint=create_pool_execute`;
  const payload = {
    initData,
    chain,
    protocol,
    token_a_address: tokenAAddress,
    token_b_address: tokenBAddress,
    fee_tier: feeTier,
    mode,
    range_mode: rangeMode,
    amount_mode: amountMode,
  };
  const wid = Number(walletId);
  if (Number.isFinite(wid) && wid > 0) payload.wallet_id = wid;
  const spacing = Number(tickSpacing);
  if (Number.isFinite(spacing) && spacing > 0) payload.tick_spacing = spacing;
  if (initialPrice !== undefined && initialPrice !== null && String(initialPrice).trim()) {
    payload.initial_price = String(initialPrice).trim();
  }
  if (minPrice !== undefined && minPrice !== null && String(minPrice).trim()) {
    payload.min_price = String(minPrice).trim();
  }
  if (maxPrice !== undefined && maxPrice !== null && String(maxPrice).trim()) {
    payload.max_price = String(maxPrice).trim();
  }
  if (amountA !== undefined && amountA !== null && String(amountA).trim()) {
    payload.amount_a = String(amountA).trim();
  }
  if (amountB !== undefined && amountB !== null && String(amountB).trim()) {
    payload.amount_b = String(amountB).trim();
  }
  if (Number.isFinite(slippageTolerance)) payload.slippage_tolerance = slippageTolerance;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  });
}

export async function setTaskPaused({ apiBaseUrl, initData, taskId, paused, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/task_action?action=pause`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData, taskId, paused: Boolean(paused) }),
    signal,
  });
}

export async function stopTask({ apiBaseUrl, initData, taskId, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/task_action?action=stop`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData, taskId }),
    signal,
  });
}

export async function deleteTask({ apiBaseUrl, initData, taskId, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/task_action?action=delete`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData, taskId }),
    signal,
  });
}

export async function updateTaskRange({ apiBaseUrl, initData, taskId, rangeLowerPct, rangeUpperPct, amountUSDT, signal }) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/task_action?action=update_range`;
  const payload = { initData, taskId, range_lower_pct: rangeLowerPct, range_upper_pct: rangeUpperPct };
  const amt = Number(amountUSDT);
  if (Number.isFinite(amt) && amt > 0) payload.amount_usdt = amt;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  });
}

export async function fetchMyTradeMarkers({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  poolId,
  bucketSec,
  startTs,
  endTs,
  windowSec = 86400,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/my_trade_markers`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      initData,
      chain,
      pool_id: poolId,
      bucket_sec: bucketSec,
      start_ts: Number.isFinite(startTs) && startTs > 0 ? Math.floor(startTs) : 0,
      end_ts: Number.isFinite(endTs) && endTs > 0 ? Math.floor(endTs) : 0,
      window_sec: windowSec,
    }),
    signal,
  });
}
