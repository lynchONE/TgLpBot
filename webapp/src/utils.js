export const WIDGETS = [
  { key: 'hot_pools', label: '热门池子' },
  { key: 'gmgn_kline', label: 'GMGN K线' },
  { key: 'positions', label: '仓位' },
  { key: 'smart_money', label: '聪明钱' },
];

export const DEFAULT_WIDGETS = WIDGETS.map((item) => item.key);

const usdFormatter = new Intl.NumberFormat('en-US', {
  style: 'currency',
  currency: 'USD',
  maximumFractionDigits: 2,
});

const usdCompactFormatter = new Intl.NumberFormat('en-US', {
  style: 'currency',
  currency: 'USD',
  notation: 'compact',
  maximumFractionDigits: 2,
});

export function formatUsd(value) {
  const n = Number(value ?? 0);
  if (!Number.isFinite(n)) return '$--';
  return usdFormatter.format(n);
}

export function formatUsdCompact(value) {
  const n = Number(value ?? 0);
  if (!Number.isFinite(n)) return '$--';
  return usdCompactFormatter.format(n);
}

export function formatPct(value, digits = 2) {
  const n = Number(value ?? 0);
  if (!Number.isFinite(n)) return '--';
  return `${n.toFixed(digits)}%`;
}

export function formatNumber(value, digits = 0) {
  const n = Number(value ?? 0);
  if (!Number.isFinite(n)) return '--';
  return n.toLocaleString('en-US', {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  });
}

export function shortAddress(value, left = 6, right = 4) {
  const raw = String(value || '').trim();
  if (!raw) return '--';
  if (raw.length <= left + right + 3) return raw;
  return `${raw.slice(0, left)}...${raw.slice(-right)}`;
}

export function normalizePoolAddress(value) {
  const raw = String(value || '').trim();
  if (!raw) return '';
  const body = raw.startsWith('0x') || raw.startsWith('0X') ? raw.slice(2) : raw;
  if (!/^[a-fA-F0-9]{40}$/.test(body) && !/^[a-fA-F0-9]{64}$/.test(body)) {
    return '';
  }
  return `0x${body.toLowerCase()}`;
}

const GMGN_STABLE_SYMBOLS = new Set([
  'usdc',
  'usdt',
  'busd',
  'dai',
  'frax',
  'usdd',
  'fdusd',
  'wbnb',
  'weth',
  'wsol',
  'bnb',
  'eth',
  'sol',
]);

function pickGmgnTokenAddress(pool) {
  const pair = String(pool?.trading_pair || pool?.pair || '').trim();
  const token0 = String(pool?.token0_address || pool?.token0 || '').trim();
  const token1 = String(pool?.token1_address || pool?.token1 || '').trim();
  if (!pair) return token0 || token1;
  const symbols = pair.split('/').map((part) => String(part || '').trim().toLowerCase());
  if (symbols.length !== 2) return token0 || token1;
  const [left, right] = symbols;
  const leftStable = GMGN_STABLE_SYMBOLS.has(left);
  const rightStable = GMGN_STABLE_SYMBOLS.has(right);
  if (leftStable && !rightStable) return token1 || token0;
  if (rightStable && !leftStable) return token0 || token1;
  return token0 || token1;
}

export function buildGmgnUrl(pool, fallbackChain = 'bsc') {
  const tokenAddress = pickGmgnTokenAddress(pool);
  if (!tokenAddress) return '';
  const chain =
    String(pool?.chain || fallbackChain || 'bsc').trim().toLowerCase() === 'base'
      ? 'base'
      : 'bsc';
  return `https://gmgn.ai/${chain}/token/${tokenAddress}`;
}

export function resolveApiBaseUrl() {
  const query = new URLSearchParams(window.location.search).get('apiBaseUrl');
  if (query && query.trim()) return query.trim();

  const envBase = String(import.meta.env.VITE_API_BASE_URL || '').trim();
  if (envBase) {
    try {
      const pageProto = window.location.protocol;
      const envProto = new URL(envBase).protocol;
      if (pageProto === 'https:' && envProto === 'http:') return '';
    } catch {
      // ignore invalid env URL
    }
    return envBase;
  }

  const host = window.location.hostname;
  if (host === 'localhost' || host === '127.0.0.1') return 'http://localhost:8080';
  return '';
}

export function resolveInitDataFromQuery() {
  const query = new URLSearchParams(window.location.search).get('initData');
  return query ? query.trim() : '';
}

export function normalizeWidgetSelection(value) {
  if (!Array.isArray(value)) return [...DEFAULT_WIDGETS];
  const allowed = new Set(DEFAULT_WIDGETS);
  const seen = new Set();
  const next = [];
  for (const raw of value) {
    const key = String(raw || '').trim();
    if (!allowed.has(key) || seen.has(key)) continue;
    seen.add(key);
    next.push(key);
  }
  return next.length ? next : [...DEFAULT_WIDGETS];
}

export function toUnixSeconds(v) {
  const n = Number(v ?? 0);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.floor(n);
}
