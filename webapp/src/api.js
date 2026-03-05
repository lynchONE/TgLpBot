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
    throw new Error(await readErrorMessage(resp));
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

  const qs = params.toString();
  const url = `${base}/api/pools?endpoint=hot_pools${qs ? `&${qs}` : ''}`;
  return requestJson(url, { method: 'GET', signal });
}

export async function fetchPoolOHLCV({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  poolAddress,
  timeframe = 'minute',
  aggregate = 5,
  limit = 240,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const params = new URLSearchParams();
  if (initData) params.set('initData', String(initData));
  if (chain) params.set('chain', String(chain));
  if (poolAddress) params.set('pool_address', String(poolAddress));
  if (timeframe) params.set('timeframe', String(timeframe));
  if (Number.isFinite(aggregate)) params.set('aggregate', String(aggregate));
  if (Number.isFinite(limit)) params.set('limit', String(limit));

  const qs = params.toString();
  const url = `${base}/api/pools?endpoint=pool_ohlcv${qs ? `&${qs}` : ''}`;
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

export async function fetchSmartMoneyOverview({
  apiBaseUrl,
  initData,
  chain = 'bsc',
  poolLimit = 20,
  walletLimit = 20,
  poolsWindowHours = 2,
  pnlWindowHours = 24,
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

export async function exchangeTelegramLogin({
  apiBaseUrl,
  id,
  first_name,
  last_name,
  username,
  photo_url,
  auth_date,
  hash,
  signal,
}) {
  const base = normalizeBaseUrl(apiBaseUrl);
  const url = `${base}/api/web_login?endpoint=telegram_login`;
  return requestJson(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      id,
      first_name,
      last_name,
      username,
      photo_url,
      auth_date,
      hash,
    }),
    signal,
  });
}
