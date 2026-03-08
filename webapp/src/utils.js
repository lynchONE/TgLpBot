export const WIDGETS = [
  { key: 'hot_pools', label: '热门池子' },
  { key: 'gmgn_kline', label: 'K线' },
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

export function normalizeHexAddress(value) {
  const raw = String(value || '').trim();
  if (!raw) return '';
  const body = raw.startsWith('0x') || raw.startsWith('0X') ? raw.slice(2) : raw;
  if (!/^[a-fA-F0-9]{40}$/.test(body)) return '';
  return `0x${body.toLowerCase()}`;
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
  const token0 = normalizeHexAddress(pool?.token0_address || pool?.token0);
  const token1 = normalizeHexAddress(pool?.token1_address || pool?.token1);
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

export function isStableLikeSymbol(symbol) {
  return GMGN_STABLE_SYMBOLS.has(String(symbol || '').trim().toLowerCase());
}

export function inferPoolVersion(pool) {
  const raw = String(pool?.pool_version || pool?.protocol_version || '').trim().toLowerCase();
  if (raw === 'v3' || raw === 'v4') return raw;
  const addr = normalizePoolAddress(pool?.pool_address || pool?.pool_id);
  if (addr && addr.length === 66) return 'v4';
  return 'v3';
}

export function resolveKlineTokenOptions(pool) {
  const token0Address = normalizeHexAddress(pool?.token0_address || pool?.token0);
  const token1Address = normalizeHexAddress(pool?.token1_address || pool?.token1);
  const pair = String(pool?.trading_pair || pool?.pair || '').trim();
  const pairSymbols = pair.includes('/') ? pair.split('/').map((part) => String(part || '').trim()) : [];
  const token0Symbol = String(pool?.token0_symbol || pairSymbols[0] || '').trim();
  const token1Symbol = String(pool?.token1_symbol || pairSymbols[1] || '').trim();

  const options = [
    token0Address
      ? {
          key: 'token0',
          address: token0Address,
          symbol: token0Symbol || 'Token0',
          isStable: isStableLikeSymbol(token0Symbol),
        }
      : null,
    token1Address
      ? {
          key: 'token1',
          address: token1Address,
          symbol: token1Symbol || 'Token1',
          isStable: isStableLikeSymbol(token1Symbol),
        }
      : null,
  ].filter(Boolean);

  if (!options.length) return { options: [], defaultKey: '' };
  if (options.length === 1) return { options, defaultKey: options[0].key };

  const stableCount = options.filter((item) => item.isStable).length;
  if (stableCount === 1) {
    const preferred = options.find((item) => !item.isStable) || options[0];
    return { options, defaultKey: preferred.key };
  }

  return { options, defaultKey: options[0].key };
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

const UTC8_OFFSET_SECONDS = 8 * 60 * 60;

function pad2(value) {
  return String(value ?? 0).padStart(2, '0');
}

function coerceUnixSeconds(value) {
  if (typeof value === 'number') {
    return toUnixSeconds(value);
  }

  if (typeof value === 'string') {
    const raw = value.trim();
    if (!raw) return 0;
    if (/^\d+$/.test(raw)) return toUnixSeconds(Number(raw));
    const parsed = Date.parse(raw);
    if (Number.isFinite(parsed) && parsed > 0) {
      return Math.floor(parsed / 1000);
    }
    return 0;
  }

  if (value && typeof value === 'object') {
    const year = Number(value.year);
    const month = Number(value.month);
    const day = Number(value.day);
    if (Number.isFinite(year) && Number.isFinite(month) && Number.isFinite(day)) {
      return Math.floor(Date.UTC(year, month - 1, day) / 1000);
    }
  }

  return 0;
}

function getUtc8DateParts(value) {
  const ts = coerceUnixSeconds(value);
  if (!ts) return null;
  const date = new Date((ts + UTC8_OFFSET_SECONDS) * 1000);
  return {
    year: date.getUTCFullYear(),
    month: date.getUTCMonth() + 1,
    day: date.getUTCDate(),
    hour: date.getUTCHours(),
    minute: date.getUTCMinutes(),
    second: date.getUTCSeconds(),
  };
}

export function formatUtc8DateTime(value, withSeconds = false) {
  const parts = getUtc8DateParts(value);
  if (!parts) return '--';
  return `${parts.year}-${pad2(parts.month)}-${pad2(parts.day)} ${pad2(parts.hour)}:${pad2(parts.minute)}${
    withSeconds ? `:${pad2(parts.second)}` : ''
  }`;
}

export function formatUtc8Time(value, withSeconds = false) {
  const parts = getUtc8DateParts(value);
  if (!parts) return '--';
  return `${pad2(parts.hour)}:${pad2(parts.minute)}${withSeconds ? `:${pad2(parts.second)}` : ''}`;
}

export function formatUtc8TickMark(value, tickMarkType) {
  const parts = getUtc8DateParts(value);
  if (!parts) return '';

  switch (Number(tickMarkType)) {
    case 0:
      return String(parts.year);
    case 1:
      return `${parts.year}-${pad2(parts.month)}`;
    case 2:
      return `${pad2(parts.month)}-${pad2(parts.day)}`;
    case 4:
      return `${pad2(parts.hour)}:${pad2(parts.minute)}:${pad2(parts.second)}`;
    case 3:
    default:
      return `${pad2(parts.hour)}:${pad2(parts.minute)}`;
  }
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

const STABLE_SYMBOLS = new Set(['USDT', 'USDC', 'BUSD', 'DAI']);

export function priceFromTick(tick, decimals0 = 18, decimals1 = 18) {
  const n = Number(tick);
  if (!Number.isFinite(n)) return null;
  const dec0 = Number(decimals0);
  const dec1 = Number(decimals1);
  if (!Number.isFinite(dec0) || !Number.isFinite(dec1)) return null;
  const v = Math.pow(1.0001, n);
  if (!Number.isFinite(v)) return null;
  const scale = Math.pow(10, dec0 - dec1);
  const adjusted = v * scale;
  return Number.isFinite(adjusted) ? adjusted : null;
}

export function computePriceRange(p) {
  const token0 = p?.token_rows?.[0];
  const token1 = p?.token_rows?.[1];
  const dec0 = Number(token0?.decimals ?? 18);
  const dec1 = Number(token1?.decimals ?? 18);
  const sym0 = String(token0?.symbol || '').trim().toUpperCase();
  const sym1 = String(token1?.symbol || '').trim().toUpperCase();
  const stableIndex = STABLE_SYMBOLS.has(sym0) ? 0 : STABLE_SYMBOLS.has(sym1) ? 1 : -1;
  const safeInvert = (v) => (Number.isFinite(v) && v > 0 ? 1 / v : null);

  const currentTick = Number(p?.current_tick);
  const tickLower = Number(p?.tick_lower);
  const tickUpper = Number(p?.tick_upper);
  const tickSpacing = Number(p?.tick_spacing);

  const currentPriceBase = priceFromTick(currentTick, dec0, dec1);
  const currentPrice = stableIndex === 0 ? safeInvert(currentPriceBase) : currentPriceBase;

  const rangeLowerBase = priceFromTick(tickLower, dec0, dec1);
  const rangeUpperBase = priceFromTick(tickUpper, dec0, dec1);
  const rangeLower = stableIndex === 0 ? safeInvert(rangeLowerBase) : rangeLowerBase;
  const rangeUpper = stableIndex === 0 ? safeInvert(rangeUpperBase) : rangeUpperBase;
  const rangeReady = Number.isFinite(rangeLower) && Number.isFinite(rangeUpper);
  const rangeMin = rangeReady ? Math.min(rangeLower, rangeUpper) : null;
  const rangeMax = rangeReady ? Math.max(rangeLower, rangeUpper) : null;

  if (!rangeReady || !Number.isFinite(currentPrice)) return null;

  const pairLabel = stableIndex === 0 ? `${sym1}/${sym0}` : `${sym0}/${sym1}`;
  const percent = rangeMax === rangeMin ? 50 : ((currentPrice - rangeMin) / (rangeMax - rangeMin)) * 100;
  const clamped = Math.max(0, Math.min(100, percent));

  const gridCount = Number.isFinite(tickLower) && Number.isFinite(tickUpper) && tickSpacing > 0
    ? Math.round(Math.abs(tickUpper - tickLower) / tickSpacing)
    : null;

  const deviation = currentPrice > 0 && rangeMin !== null && rangeMax !== null
    ? ((Math.max(0, (rangeMax / currentPrice) - 1) * 100) + (Math.max(0, 1 - (rangeMin / currentPrice)) * 100)) / 2
    : null;

  let outOfRange = null;
  if (currentPrice > rangeMax) {
    const base = Math.abs(rangeMax) > 0 ? Math.abs(rangeMax) : 1;
    outOfRange = { direction: 'above', pct: ((currentPrice - rangeMax) / base) * 100 };
  } else if (currentPrice < rangeMin) {
    const base = Math.abs(rangeMin) > 0 ? Math.abs(rangeMin) : 1;
    outOfRange = { direction: 'below', pct: ((rangeMin - currentPrice) / base) * 100 };
  }

  const visibleGridLines = [];
  if (gridCount && gridCount >= 2 && gridCount <= 200) {
    const maxLines = Math.min(gridCount - 1, 40);
    const step = Math.ceil((gridCount - 1) / maxLines);
    for (let i = 1; i < gridCount; i++) {
      if ((gridCount - 1) <= 40 || i % step === 0) {
        visibleGridLines.push((i / gridCount) * 100);
      }
    }
  }

  return { currentPrice, rangeMin, rangeMax, pairLabel, percent: clamped, inRange: Boolean(p?.in_range), gridCount, deviation, outOfRange, visibleGridLines };
}

export function formatDuration(isoString) {
  if (!isoString) return '';
  const ts = Date.parse(isoString);
  if (!Number.isFinite(ts)) return '';
  const diffSec = Math.max(0, Math.floor((Date.now() - ts) / 1000));
  const min = Math.floor(diffSec / 60);
  if (min < 60) return `${min}分钟`;
  const h = Math.floor(min / 60);
  const remMin = min % 60;
  if (h < 24) return remMin ? `${h}h${remMin}m` : `${h}h`;
  const d = Math.floor(h / 24);
  const remH = h % 24;
  return remH ? `${d}d${remH}h` : `${d}d`;
}
