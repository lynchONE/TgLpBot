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

export function pickNonStableTokenAddress(pool) {
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
  const tokenAddress = pickNonStableTokenAddress(pool);
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

export function formatPriceDisplay(raw) {
  const s = String(raw || '').trim();
  if (!s) return '';
  // e.g. "1 ASTER = 0.71192319 USD" → shorten the number
  const m = s.match(/^(1\s+\w+\s*=\s*)([0-9.]+)(\s*USD)$/i);
  if (!m) return s;
  const n = Number(m[2]);
  if (!Number.isFinite(n)) return s;
  let formatted;
  if (n >= 1000) {
    formatted = n.toLocaleString('en-US', { maximumFractionDigits: 2 });
  } else if (n >= 1) {
    formatted = n.toFixed(4);
  } else if (n >= 0.01) {
    formatted = n.toFixed(6);
  } else if (n === 0) {
    formatted = '0';
  } else {
    // For very small numbers like 0.00000038, use subscript notation: 0.0₇38
    const str = n.toFixed(20).replace(/0+$/, '');
    const afterDot = str.split('.')[1] || '';
    const leadingZeros = afterDot.match(/^0*/)[0].length;
    if (leadingZeros >= 3) {
      const significant = afterDot.slice(leadingZeros, leadingZeros + 4).replace(/0+$/, '') || '0';
      const sub = String(leadingZeros).split('').map(c => '₀₁₂₃₄₅₆₇₈₉'[Number(c)]).join('');
      formatted = `0.0${sub}${significant}`;
    } else {
      formatted = n.toFixed(Math.min(8, leadingZeros + 4));
    }
  }
  return `${m[1]}${formatted}${m[3]}`;
}

export function toUnixSeconds(v) {
  const n = Number(v ?? 0);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.floor(n);
}

const SUBSCRIPT_DIGITS = ['₀', '₁', '₂', '₃', '₄', '₅', '₆', '₇', '₈', '₉'];
export function compactPrice(v) {
  const n = Number(v ?? 0);
  if (!Number.isFinite(n) || n <= 0) return '--';
  if (n >= 1000) return n.toLocaleString('en-US', { maximumFractionDigits: 2 });
  if (n >= 1) return n.toFixed(4).replace(/\.?0+$/, '');
  if (n >= 0.01) return n.toPrecision(4);
  const s = n.toFixed(20);
  const dotIdx = s.indexOf('.');
  if (dotIdx < 0) return n.toPrecision(4);
  let zeroCount = 0;
  for (let i = dotIdx + 1; i < s.length; i++) {
    if (s[i] === '0') zeroCount++;
    else break;
  }
  if (zeroCount < 2) return n.toPrecision(4);
  const sigStart = dotIdx + 1 + zeroCount;
  const sigDigits = s.slice(sigStart, sigStart + 4).replace(/0+$/, '') || '0';
  const sub = String(zeroCount).split('').map(d => SUBSCRIPT_DIGITS[Number(d)] || d).join('');
  return `0.0${sub}${sigDigits}`;
}
