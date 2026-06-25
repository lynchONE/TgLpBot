/**
 * 统一的格式化工具集。
 *
 * 来源：合并 App.jsx / PositionCard.jsx / SmartMoneyPage.jsx 三处重复实现。
 * 行为以 App.jsx 为基准；SmartMoneyPage 的 `—` 占位通过 `fallback` 选项支持。
 */

export const USD_DISPLAY_LIMIT = 1e15;

export const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

const botAmountFormatter = new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
    useGrouping: false,
});

export function formatUsd(v, { fallback = '$--' } = {}) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return fallback;
    return usdFormatter.format(n);
}

export function formatFeeUsd(v, { fallback = '$--' } = {}) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return fallback;
    if (n === 0) return usdFormatter.format(0);
    const abs = Math.abs(n);
    if (abs < 0.01) return `${n < 0 ? '-' : ''}<$0.01`;
    return usdFormatter.format(n);
}

export function formatBotAmount(v, { fallback = '--' } = {}) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return fallback;
    if (Math.abs(n) < 0.005) return '0.00';
    return botAmountFormatter.format(n);
}

export function formatUsdCompact(v, { fallback = '$--' } = {}) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || n <= 0 || Math.abs(n) > USD_DISPLAY_LIMIT) return fallback;
    const abs = Math.abs(n);
    if (abs >= 1000000) return `$${(n / 1000000).toFixed(abs >= 10000000 ? 0 : 1).replace(/\.0$/, '')}M`;
    if (abs >= 1000) return `$${(n / 1000).toFixed(abs >= 10000 ? 0 : 1).replace(/\.0$/, '')}K`;
    if (abs >= 100) return `$${n.toFixed(0)}`;
    if (abs >= 10) return `$${n.toFixed(1).replace(/\.0$/, '')}`;
    return `$${n.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
}

export {
    normalizeHexAddress,
    normalizeTokenRisk,
    shortAddress,
    tokenRiskLabel,
    tokenRiskSummary,
    tokenRiskToneClass,
} from '../../../shared/frontend/tokenRisk.js';

export function formatRangePercent(value, { fallback = '--' } = {}) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return fallback;
    if (num >= 100) return `${Math.round(num)}%`;
    if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

export function formatSignedPercent(value, { fallback = '--' } = {}) {
    const num = Number(value);
    if (!Number.isFinite(num)) return fallback;
    if (Math.abs(num) < 0.0001) return '0%';
    return `${num > 0 ? '+' : '-'}${formatRangePercent(Math.abs(num))}`;
}

export function formatPercentInputValue(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '';
    if (num >= 10) return num.toFixed(1).replace(/\.0$/, '');
    if (num >= 1) return num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '');
    return num.toFixed(3).replace(/0+$/, '').replace(/\.$/, '');
}

export function formatPrice(value, { fallback = '--' } = {}) {
    const n = Number(value);
    if (!Number.isFinite(n)) return fallback;
    if (n === 0) return '0';
    const sign = n < 0 ? '-' : '';
    let s = Math.abs(n).toFixed(18).replace(/\.?0+$/, '');
    if (!s.includes('.')) return `${sign}${s}`;
    const [intPart, fracRaw] = s.split('.');
    const frac = fracRaw || '';
    let nonZero = 0;
    let cut = frac.length;
    for (let i = 0; i < frac.length; i++) {
        if (frac[i] !== '0') {
            nonZero += 1;
            if (nonZero === 2) { cut = i + 1; break; }
        }
    }
    const trimmed = frac.slice(0, cut);
    return trimmed ? `${sign}${intPart}.${trimmed}` : `${sign}${intPart}`;
}

export function formatPriceValue(value, { fallback = '--' } = {}) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return fallback;
    if (num >= 1000) return num.toLocaleString(undefined, { maximumFractionDigits: 2 });
    if (num >= 1) return num.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 });
    return Number(num.toPrecision(6)).toString();
}

export function formatPriceInputValue(value) {
    const text = formatPriceValue(value);
    return text === '--' ? '' : text;
}

export function formatFeeTier(fee) {
    const n = Number(fee || 0);
    if (!Number.isFinite(n) || n <= 0 || n > 1000000) return '';
    return `${(n / 10000).toFixed(4)}%`;
}

export function isDynamicFeeTier(fee) {
    const n = Number(fee);
    return Number.isInteger(n) && (n & 0x800000) !== 0;
}

export function formatWalletBalance(value, { fallback = '--' } = {}) {
    const num = Number(value);
    if (!Number.isFinite(num)) return fallback;
    if (num === 0) return '$0';
    return formatUsdCompact(num);
}

/* ---- 兼容别名（保留旧调用点） ---- */
export const formatRangePercentCompact = formatRangePercent;
export const formatRangePercentPlain = formatRangePercent;
export const formatSignedPercentCompact = formatSignedPercent;
export const formatUSDCompact = (value) => formatUsdCompact(value, { fallback: '—' });
