function normalizeBaseUrl(apiBaseUrl) {
  return String(apiBaseUrl || '').trim().replace(/\/$/, '');
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
  const url = `${base}/api/pools?endpoint=hot_pools${qs ? `&${qs}` : ''}`;
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

export async function fetchSmartMoneyOverview({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  poolLimit = 20,
  walletLimit = 20,
  poolsWindowHours = 2,
  pnlWindowHours = 2,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const params = new URLSearchParams();
  if (initData) params.set('initData', String(initData));
  if (chain) params.set('chain', String(chain));
  if (Number.isFinite(poolLimit)) params.set('pool_limit', String(poolLimit));
  if (Number.isFinite(walletLimit))
    params.set('wallet_limit', String(walletLimit));
  if (Number.isFinite(poolsWindowHours))
    params.set('pools_window_hours', String(poolsWindowHours));
  if (Number.isFinite(pnlWindowHours))
    params.set('pnl_window_hours', String(pnlWindowHours));

  const qs = params.toString();
  const url = `${base}/api/smart_money${qs ? `?${qs}` : ''}`;
  return requestJson(url, { method: 'GET', signal });
}

export async function fetchSmartMoneyPoolAdds({
  apiBaseUrl,
  initData,
  chain,
  poolVersion,
  poolId,
  windowHours,
  limit,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const params = new URLSearchParams();
  if (initData) params.set('initData', String(initData));
  if (chain) params.set('chain', String(chain));
  if (poolVersion) params.set('pool_version', String(poolVersion));
  if (poolId) params.set('pool_id', String(poolId));
  if (Number.isFinite(windowHours)) params.set('window_hours', String(windowHours));
  if (Number.isFinite(limit)) params.set('limit', String(limit));
  const qs = params.toString();
  const url = `${base}/api/smart_money_pool_adds${qs ? `?${qs}` : ''}`;
  return requestJson(url, { method: 'GET', signal });
}

export async function fetchSmartMoneyPoolMarkers({
  apiBaseUrl,
  initData,
  chain,
  poolVersion,
  poolId,
  bucketSec,
  windowHours,
  startTs,
  endTs,
  limit,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const params = new URLSearchParams();
  if (initData) params.set('initData', String(initData));
  if (chain) params.set('chain', String(chain));
  if (poolVersion) params.set('pool_version', String(poolVersion));
  if (poolId) params.set('pool_id', String(poolId));
  if (Number.isFinite(bucketSec)) params.set('bucket_sec', String(bucketSec));
  if (Number.isFinite(windowHours)) params.set('window_hours', String(windowHours));
  if (Number.isFinite(startTs) && startTs > 0) params.set('start_ts', String(Math.floor(startTs)));
  if (Number.isFinite(endTs) && endTs > 0) params.set('end_ts', String(Math.floor(endTs)));
  if (Number.isFinite(limit)) params.set('limit', String(limit));

  const qs = params.toString();
  const url = `${base}/api/smart_money_pool_markers${qs ? `?${qs}` : ''}`;
  return requestJson(url, { method: 'GET', signal });
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

export async function fetchSmartMoneyWatchedWallets({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const params = new URLSearchParams();
  if (initData) params.set('initData', String(initData));
  if (chain) params.set('chain', String(chain));
  const qs = params.toString();
  const url = `${base}/api/smart_money_watched_wallets${qs ? `?${qs}` : ''}`;
  return requestJson(url, { method: 'GET', signal });
}

export async function addSmartMoneyWatchedWallets({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  wallets,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/smart_money_watched_wallets`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData, chain, wallets }),
    signal,
  });
}

export async function removeSmartMoneyWatchedWallets({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  walletAddresses,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const params = new URLSearchParams();
  if (initData) params.set('initData', String(initData));
  if (chain) params.set('chain', String(chain));
  const normalizedWalletAddresses = Array.isArray(walletAddresses)
    ? walletAddresses.map((item) => String(item || '').trim()).filter(Boolean)
    : [];
  if (normalizedWalletAddresses.length > 0) {
    params.set('wallet_addresses', normalizedWalletAddresses.join(','));
  }
  const qs = params.toString();
  const url = `${base}/api/smart_money_watched_wallets${qs ? `?${qs}` : ''}`;
  return requestJson(url, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData, chain, wallet_addresses: normalizedWalletAddresses }),
    signal,
  });
}
