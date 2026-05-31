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

export function normalizeHexAddress(value) {
    const raw = String(value || '').trim();
    if (!raw) return '';
    const body = raw.startsWith('0x') || raw.startsWith('0X') ? raw.slice(2) : raw;
    if (!/^[a-fA-F0-9]{40}$/.test(body)) return '';
    return `0x${body.toLowerCase()}`;
}

export function shortAddress(value, left = 6, right = 4) {
    const raw = String(value || '').trim();
    if (!raw) return '--';
    if (raw.length <= left + right + 3) return raw;
    return `${raw.slice(0, left)}...${raw.slice(-right)}`;
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
    if (!fee) return '';
    return `${(Number(fee) / 10000).toFixed(4)}%`;
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
