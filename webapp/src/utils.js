export const WIDGETS = [
  { key: 'create_pool', label: '创建池子' },
  { key: 'hot_pools', label: '热门池子' },
  { key: 'gmgn_kline', label: 'K线' },
  { key: 'positions', label: '仓位' },
  { key: 'assets', label: '我的' },
  { key: 'smart_money', label: '聪明钱' },
  { key: 'swap', label: '一键兑换' },
  { key: 'admin_panel', label: '管理员' },
];

export const DEFAULT_WIDGETS = WIDGETS.map((item) => item.key);

export const WIDGET_MODULE_MAP = {
  create_pool: 'create_pool',
  hot_pools: 'hot_pools',
  gmgn_kline: 'gmgn_kline',
  positions: 'positions',
  assets: 'assets',
  smart_money: 'smart_money',
  swap: 'swap',
  admin_panel: 'admin_panel',
};

export function normalizeEnabledModules(value) {
  if (!Array.isArray(value)) return [];
  const seen = new Set();
  const modules = [];
  value.forEach((item) => {
    const key = String(item || '').trim();
    if (!key || seen.has(key)) return;
    seen.add(key);
    modules.push(key);
  });
  return modules;
}

export function normalizeAccessInfo(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  return {
    allowed: Boolean(value.allowed),
    is_admin: Boolean(value.is_admin),
    mini_app_enabled: Boolean(value.mini_app_enabled),
    enabled_modules: normalizeEnabledModules(value.enabled_modules),
    module_catalog: Array.isArray(value.module_catalog) ? value.module_catalog : [],
  };
}

export function canAccessWidget(widgetKey, accessInfo) {
  const key = String(widgetKey || '').trim();
  if (!key || !accessInfo) return false;
  if (key === 'admin_panel') return Boolean(accessInfo.is_admin);
  if (accessInfo.is_admin) return true;
  if (!accessInfo.mini_app_enabled) return false;
  const moduleKey = WIDGET_MODULE_MAP[key];
  if (!moduleKey || moduleKey === 'admin_panel') return false;
  return normalizeEnabledModules(accessInfo.enabled_modules).includes(moduleKey);
}

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

const TOKEN_RISK_LEVEL_LABELS = ['未定义', '低', '中', '中高', '高', '高(人工)'];

function tokenRiskLevelToChinese(value) {
  const raw = String(value || '').trim();
  switch (raw.toLowerCase()) {
    case 'undefined':
      return '未定义';
    case 'low':
      return '低';
    case 'medium':
      return '中';
    case 'medium-high':
      return '中高';
    case 'high':
      return '高';
    case 'high(manual)':
      return '高(人工)';
    default:
      return raw;
  }
}

function tokenRiskWarningToChinese(value) {
  const raw = String(value || '').trim();
  const lower = raw.toLowerCase();
  if (!raw) return '';
  if (lower.includes('okx marked honeypot')) return 'OKX 标记为貔貅盘';
  if (lower.includes('okx marked low liquidity')) return 'OKX 标记为低流动性';
  if (lower.startsWith('okx risk level:')) {
    return `OKX 风险等级: ${tokenRiskLevelToChinese(raw.split(':').slice(1).join(':'))}`;
  }
  if (lower.startsWith('okx risk lookup failed:')) {
    return `OKX 风控查询失败: ${raw.split(':').slice(1).join(':').trim()}`;
  }
  if (lower.includes('429') || lower.includes('too many')) return 'OKX 风控接口限流，已延后后台刷新';
  if (lower.includes('advanced-info returned empty data')) return 'OKX advanced-info 未返回风控数据';
  if (lower.includes('low liquidity')) return raw.replace(/low liquidity/ig, '低流动性');
  if (lower.includes('honeypot')) return raw.replace(/honeypot/ig, '貔貅盘');
  return raw;
}

export function normalizeTokenRisk(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  const level = Number(value.risk_control_level);
  const warnings = Array.isArray(value.warnings)
    ? value.warnings.map(tokenRiskWarningToChinese).filter(Boolean)
    : [];
  const tags = Array.isArray(value.token_tags)
    ? value.token_tags.map((item) => String(item || '').trim()).filter(Boolean)
    : [];
  return {
    ...value,
    risk_control_level: Number.isFinite(level) ? level : 0,
    risk_control_label: TOKEN_RISK_LEVEL_LABELS[Number.isFinite(level) ? level : 0] || tokenRiskLevelToChinese(value.risk_control_label) || '未知',
    risk_tone: String(value.risk_tone || '').trim() || 'unknown',
    token_symbol: String(value.token_symbol || '').trim(),
    token_address: normalizeHexAddress(value.token_address),
    has_honeypot: Boolean(value.has_honeypot),
    has_low_liquidity: Boolean(value.has_low_liquidity),
    warnings,
    token_tags: tags,
    error: tokenRiskWarningToChinese(value.error),
  };
}

export function tokenRiskToneClass(risk) {
  const normalized = normalizeTokenRisk(risk);
  if (!normalized) return 'unknown';
  if (normalized.has_honeypot) return 'critical';
  switch (normalized.risk_tone) {
    case 'critical':
    case 'high':
    case 'medium':
    case 'low':
    case 'neutral':
    case 'unknown':
      return normalized.risk_tone;
    default:
      return 'unknown';
  }
}

export function tokenRiskLabel(risk) {
  const normalized = normalizeTokenRisk(risk);
  if (!normalized) return '';
  if (normalized.has_honeypot) return '貔貅盘';
  if (normalized.has_low_liquidity) return '低流动性';
  if (normalized.error) return '风控未知';
  return `风险 ${normalized.risk_control_label}`;
}

export function tokenRiskSummary(risk) {
  const normalized = normalizeTokenRisk(risk);
  if (!normalized) return '';
  if (normalized.warnings.length > 0) return normalized.warnings.join('；');
  const symbol = normalized.token_symbol || shortAddress(normalized.token_address);
  return `${symbol ? `${symbol} ` : ''}OKX 风控等级: ${normalized.risk_control_label}`;
}

export function computeHotPoolActiveFeeRate(pool) {
  const totalFees = Number(pool?.total_fees ?? 0);
  const activeLiquidityUsd = Number(pool?.activeLiquidityUSD ?? pool?.active_liquidity_usd ?? 0);
  if (!Number.isFinite(totalFees) || !Number.isFinite(activeLiquidityUsd) || activeLiquidityUsd <= 0) {
    return null;
  }
  return (totalFees / activeLiquidityUsd) * 100;
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

export function resolveHotPoolFilterToken(pool) {
  const pair = String(pool?.trading_pair || pool?.pair || '').trim();
  const token0Address = normalizeHexAddress(pool?.token0_address || pool?.token0);
  const token1Address = normalizeHexAddress(pool?.token1_address || pool?.token1);
  const pairSymbols = pair.includes('/')
    ? pair.split('/').map((part) => String(part || '').trim())
    : [];
  const token0Symbol = String(pool?.token0_symbol || pairSymbols[0] || '').trim();
  const token1Symbol = String(pool?.token1_symbol || pairSymbols[1] || '').trim();
  const token0Stable = isStableLikeSymbol(token0Symbol);
  const token1Stable = isStableLikeSymbol(token1Symbol);

  if (token0Stable === token1Stable) return null;
  if (token0Stable && !token1Stable && token1Address) {
    return { address: token1Address, symbol: token1Symbol || 'Token' };
  }
  if (token1Stable && !token0Stable && token0Address) {
    return { address: token0Address, symbol: token0Symbol || 'Token' };
  }
  return null;
}

function normalizeHotPoolBadgeText(value) {
  if (typeof value === 'string' || typeof value === 'number') {
    return String(value).trim();
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return '';
  }

  const orderedKeys = ['label', 'text', 'title', 'name', 'badge', 'content', 'value', 'type', 'tip'];
  for (const key of orderedKeys) {
    const candidate = value[key];
    if (typeof candidate === 'string' || typeof candidate === 'number') {
      const label = String(candidate).trim();
      if (label) return label;
    }
  }

  for (const candidate of Object.values(value)) {
    if (typeof candidate === 'string' || typeof candidate === 'number') {
      const label = String(candidate).trim();
      if (label) return label;
    }
  }

  return '';
}

function normalizeHotPoolBadgeTip(value, fallbackText) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return fallbackText;
  }

  const orderedKeys = ['tip', 'tooltip', 'description', 'desc', 'detail', 'title', 'text', 'label'];
  for (const key of orderedKeys) {
    const candidate = value[key];
    if (typeof candidate === 'string' || typeof candidate === 'number') {
      const tip = String(candidate).trim();
      if (tip) return tip;
    }
  }

  return fallbackText;
}

export function parseHotPoolBadges(value, limit = 6) {
  let source = value;
  if (typeof source === 'string') {
    const raw = source.trim();
    if (!raw) return [];
    try {
      source = JSON.parse(raw);
    } catch {
      source = [raw];
    }
  }

  if (!Array.isArray(source) || !source.length) return [];

  const badges = [];
  const seen = new Set();
  for (const item of source) {
    const text = normalizeHotPoolBadgeText(item);
    if (!text) continue;
    const tip = normalizeHotPoolBadgeTip(item, text);
    const normalized = `${text.toLowerCase()}::${tip.toLowerCase()}`;
    if (seen.has(normalized)) continue;
    seen.add(normalized);
    badges.push({ text, tip });
    if (badges.length >= limit) break;
  }
  return badges;
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
  const safeInvert = (v) => (Number.isFinite(v) && v > 0 ? 1 / v : null);
  const useInvertedPrice = isStableLikeSymbol(sym0) && !isStableLikeSymbol(sym1);

  const currentTick = Number(p?.current_tick);
  const tickLower = Number(p?.tick_lower);
  const tickUpper = Number(p?.tick_upper);
  const tickSpacing = Number(p?.tick_spacing);

  const currentPriceBase = priceFromTick(currentTick, dec0, dec1);
  const currentPrice = useInvertedPrice ? safeInvert(currentPriceBase) : currentPriceBase;

  const rangeLowerBase = priceFromTick(tickLower, dec0, dec1);
  const rangeUpperBase = priceFromTick(tickUpper, dec0, dec1);
  const rangeLower = useInvertedPrice ? safeInvert(rangeLowerBase) : rangeLowerBase;
  const rangeUpper = useInvertedPrice ? safeInvert(rangeUpperBase) : rangeUpperBase;
  const rangeReady = Number.isFinite(rangeLower) && Number.isFinite(rangeUpper);
  const rangeMin = rangeReady ? Math.min(rangeLower, rangeUpper) : null;
  const rangeMax = rangeReady ? Math.max(rangeLower, rangeUpper) : null;

  if (!rangeReady || !Number.isFinite(currentPrice)) return null;

  const pairLabel = useInvertedPrice ? `${sym1}/${sym0}` : `${sym0}/${sym1}`;
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
