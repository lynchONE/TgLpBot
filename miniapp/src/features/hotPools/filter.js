import { normalizeTokenRisk } from '../../lib/format';

export const STORAGE_HOT_POOLS_FILTER = 'tglp_hot_pools_filter_v1';

export const defaultHotPoolsFilter = {
    enabled: true,
    keyword: '',
    riskFilter: 'all',
    minFees: 60,
    minFeeRate: 0.3,
    maxFeeRate: null,
    minActiveFeeRate: null,
    minTvl: 1000,
    minMarketCap: null,
    minVolume: 2000,
    minTxCount: null,
};

export const HOT_POOLS_RISK_FILTER_ALL = 'all';

export const HOT_POOLS_RISK_FILTER_OPTIONS = [
    { value: HOT_POOLS_RISK_FILTER_ALL, label: '全部' },
    { value: 'exclude_low_liquidity', label: '排除低流动性' },
    { value: 'only_low_liquidity', label: '仅低流动性' },
];

export const HOT_POOL_SORT_TABS = [
    { key: 'fees', label: '手续费' },
    { key: 'fee_rate', label: '费率' },
    { key: 'volume', label: '交易量' },
];

export function normalizeHotPoolsRiskFilter(value) {
    const key = String(value || '').trim();
    return HOT_POOLS_RISK_FILTER_OPTIONS.some((item) => item.value === key)
        ? key
        : HOT_POOLS_RISK_FILTER_ALL;
}

export function parseNullableNumber(value) {
    if (value === null || value === undefined || value === '') return null;
    const n = Number(value);
    if (!Number.isFinite(n)) return null;
    return Math.max(0, n);
}

export function parseMetricNumber(value) {
    if (value === null || value === undefined || value === '') return NaN;
    const raw = typeof value === 'string' ? value.replace(/,/g, '').trim() : value;
    const direct = Number(raw);
    if (Number.isFinite(direct)) return direct;
    const match = String(value).match(/-?\d+(\.\d+)?/);
    if (!match) return NaN;
    const parsed = Number(match[0]);
    return Number.isFinite(parsed) ? parsed : NaN;
}

export function computeHotPoolActiveFeeRate(pool) {
    const totalFees = Number(pool?.total_fees ?? 0);
    const activeLiquidityUsd = Number(pool?.activeLiquidityUSD ?? pool?.active_liquidity_usd ?? 0);
    if (!Number.isFinite(totalFees) || !Number.isFinite(activeLiquidityUsd) || activeLiquidityUsd <= 0) {
        return null;
    }
    return (totalFees / activeLiquidityUsd) * 100;
}

export function resolveHotPoolMarketCap(pool) {
    const fdv = parseMetricNumber(pool?.fdv_usd);
    if (Number.isFinite(fdv) && fdv > 0) return fdv;
    return parseMetricNumber(pool?.current_token_fdv_usd);
}

export function resolveHotPoolMarketCapDisplay(pool) {
    const candidates = [
        pool?.fdv_usd,
        pool?.current_token_fdv_usd,
        pool?.market_cap_usd,
    ];
    for (const candidate of candidates) {
        const value = parseMetricNumber(candidate);
        if (Number.isFinite(value) && value > 0) return value;
    }
    return NaN;
}

export function resolveHotPoolMarketCapLabel(pool) {
    const fdv = parseMetricNumber(pool?.fdv_usd);
    if (Number.isFinite(fdv) && fdv > 0) return 'FDV';
    const legacyFDV = parseMetricNumber(pool?.current_token_fdv_usd);
    if (Number.isFinite(legacyFDV) && legacyFDV > 0) return 'FDV';
    return '市值';
}

export function normalizeHotPoolsFilter(value) {
    const base = { ...defaultHotPoolsFilter };
    if (!value || typeof value !== 'object') return base;
    if (Object.prototype.hasOwnProperty.call(value, 'enabled')) {
        base.enabled = Boolean(value.enabled);
    }
    if (Object.prototype.hasOwnProperty.call(value, 'keyword')) {
        const raw = String(value.keyword ?? '').trim();
        base.keyword = raw.length > 64 ? raw.slice(0, 64) : raw;
    }
    if (Object.prototype.hasOwnProperty.call(value, 'riskFilter')) {
        base.riskFilter = normalizeHotPoolsRiskFilter(value.riskFilter);
    }
    if (Object.prototype.hasOwnProperty.call(value, 'minFees')) {
        base.minFees = parseNullableNumber(value.minFees);
    }
    if (Object.prototype.hasOwnProperty.call(value, 'minFeeRate')) {
        base.minFeeRate = parseNullableNumber(value.minFeeRate);
    }
    if (Object.prototype.hasOwnProperty.call(value, 'maxFeeRate')) {
        base.maxFeeRate = parseNullableNumber(value.maxFeeRate);
    }
    if (Object.prototype.hasOwnProperty.call(value, 'minActiveFeeRate')) {
        base.minActiveFeeRate = parseNullableNumber(value.minActiveFeeRate);
    }
    if (Object.prototype.hasOwnProperty.call(value, 'minTvl')) {
        base.minTvl = parseNullableNumber(value.minTvl);
    }
    if (Object.prototype.hasOwnProperty.call(value, 'minMarketCap')) {
        base.minMarketCap = parseNullableNumber(value.minMarketCap);
    }
    if (Object.prototype.hasOwnProperty.call(value, 'minVolume')) {
        base.minVolume = parseNullableNumber(value.minVolume);
    }
    if (Object.prototype.hasOwnProperty.call(value, 'minTxCount')) {
        base.minTxCount = parseNullableNumber(value.minTxCount);
    }
    return base;
}

export function parseDraftNumber(raw) {
    const text = String(raw || '').trim();
    if (!text) return null;
    const match = text.match(/-?\d+(\.\d+)?/);
    if (!match) return null;
    const n = Number(match[0]);
    if (!Number.isFinite(n)) return null;
    return Math.max(0, n);
}

export function formatDraftNumber(value) {
    return Number.isFinite(value) ? String(value) : '';
}

export function hotPoolMatchesRiskFilter(pool, filterKey) {
    const key = normalizeHotPoolsRiskFilter(filterKey);
    if (key === HOT_POOLS_RISK_FILTER_ALL) return true;

    const risk = normalizeTokenRisk(pool?.token_risk);
    const isLowLiquidity = Boolean(risk?.has_low_liquidity);

    switch (key) {
        case 'exclude_low_liquidity':
            return !isLowLiquidity;
        case 'only_low_liquidity':
            return isLowLiquidity;
        default:
            return true;
    }
}
